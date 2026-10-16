package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
)

type OrderRepository struct{ db *gorm.DB }

func NewOrderRepository(db *gorm.DB) *OrderRepository { return &OrderRepository{db: db} }

// Save writes the order, its items and its discounts in one transaction: if
// any row is rejected (for example an unknown product), nothing is stored.
func (r *OrderRepository) Save(ctx context.Context, o domain.Order) error {
	model := toOrderModel(o)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&model).Error // inserts Items and Discounts too
	})
	if err != nil {
		return fmt.Errorf("save order: %w", err)
	}
	return nil
}

func toOrderModel(o domain.Order) orderModel {
	m := orderModel{
		ID:                   o.ID,
		CustomerID:           o.CustomerID,
		TotalBeforeSatang:    int64(o.Quote.TotalBefore),
		MemberDiscountSatang: int64(o.Quote.MemberDiscount()),
		FinalTotalSatang:     int64(o.Quote.FinalTotal),
		CreatedAt:            o.CreatedAt,
	}
	if o.MemberCard != "" {
		card := o.MemberCard
		m.MemberCard = &card
	}
	for _, l := range o.Lines {
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
