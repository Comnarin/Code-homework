package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
)

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// Save enforces the active product limits and writes the order, its items and
// its discounts in one transaction: if any row is rejected (for example an
// unknown product), nothing is stored.
//
// Limit enforcement is what makes Postgres the authority on the Red rule. In
// the same transaction it locks the active product_limits rows of the ordered
// products (in product-ID order, so concurrent multi-product orders cannot
// deadlock), then looks up who opened each window. A different holder inside
// the window gets *domain.SlotHeldError and nothing is written. The holder
// that opens a window has its item flagged started_limit_window; a repeat
// order by the same holder is allowed but is not flagged, so it never extends
// the window. The anchor lookup is a separate statement after the lock so
// that, under READ COMMITTED, it sees every order committed before the lock
// was granted.
func (r *OrderRepository) Save(ctx context.Context, o domain.Order) error {
	model := toOrderModel(o)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := enforceLimits(tx, o, &model); err != nil {
			return err
		}
		return tx.Create(&model).Error // inserts Items and Discounts too
	})
	if err != nil {
		var held *domain.SlotHeldError
		if errors.As(err, &held) {
			return err
		}
		return fmt.Errorf("save order: %w", err)
	}
	return nil
}

type limitRow struct {
	ProductID     string
	WindowSeconds int
}

func enforceLimits(tx *gorm.DB, o domain.Order, model *orderModel) error {
	if len(o.Carts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(o.Carts))
	names := make(map[string]string, len(o.Carts))
	for _, c := range o.Carts {
		id := string(c.Product.ID)
		ids = append(ids, id)
		names[id] = c.Product.Name
	}
	sort.Strings(ids)

	var limits []limitRow
	if err := tx.Raw(`SELECT product_id, window_seconds FROM product_limits
		WHERE active AND product_id IN ? ORDER BY product_id FOR UPDATE`, ids).Scan(&limits).Error; err != nil {
		return fmt.Errorf("lock limits: %w", err)
	}

	holder := o.Holder()
	for _, l := range limits {
		since := o.CreatedAt.Add(-time.Duration(l.WindowSeconds) * time.Second)
		var holders []string
		if err := tx.Raw(`SELECT COALESCE(o.client_key, o.id::text)
			FROM order_items i JOIN orders o ON o.id = i.order_id
			WHERE i.product_id = ? AND i.started_limit_window AND o.created_at > ?
			ORDER BY o.created_at DESC LIMIT 1`, l.ProductID, since).Scan(&holders).Error; err != nil {
			return fmt.Errorf("find limit anchor: %w", err)
		}
		switch {
		case len(holders) == 0:
			for i := range model.Items {
				if model.Items[i].ProductID == l.ProductID {
					model.Items[i].StartedLimitWindow = true
				}
			}
		case holders[0] != holder:
			return &domain.SlotHeldError{ProductID: domain.ProductID(l.ProductID), ProductName: names[l.ProductID]}
		}
	}
	return nil
}

// ClearLimitWindows forgets every open limit window, so limited products can
// be ordered again by anyone (test aid). Orders themselves are kept.
func (r *OrderRepository) ClearLimitWindows(ctx context.Context) error {
	if err := r.db.WithContext(ctx).Exec(`UPDATE order_items SET started_limit_window = false WHERE started_limit_window`).Error; err != nil {
		return fmt.Errorf("clear limit windows: %w", err)
	}
	return nil
}

func toOrderModel(o domain.Order) orderModel {
	m := orderModel{
		ID:                   o.ID,
		TotalBeforeSatang:    int64(o.Quote.TotalBefore),
		MemberDiscountSatang: int64(o.Quote.MemberDiscount()),
		FinalTotalSatang:     int64(o.Quote.FinalTotal),
		CreatedAt:            o.CreatedAt,
	}
	if o.ClientKey != "" {
		key := o.ClientKey
		m.ClientKey = &key
	}
	if o.MemberCard != "" {
		card := o.MemberCard
		m.MemberCard = &card
	}
	for _, l := range o.Carts {
		m.Items = append(m.Items, orderItemModel{OrderID: o.ID, ProductID: string(l.Product.ID), Quantity: l.Quantity})
	}
	for _, d := range o.Quote.Discounts {
		row := orderDiscountModel{OrderID: o.ID, RuleCode: d.RuleCode, AmountSatang: int64(d.Amount)}
		if d.ProductID != "" {
			id := string(d.ProductID)
			row.ProductID = &id
		}
		m.Discounts = append(m.Discounts, row)
	}
	return m
}
