// Package postgres implements the use-case ports on Postgres via GORM.
//
// The models below are persistence shapes only. They carry the GORM tags so
// the domain types stay free of any database concern; each repository maps
// between the two.
package postgres

import "time"

type productModel struct {
	ID          string `gorm:"column:id;primaryKey"`
	Name        string `gorm:"column:name"`
	PriceSatang int64  `gorm:"column:price_satang"`
	SortOrder   int    `gorm:"column:sort_order"`
}

func (productModel) TableName() string { return "products" }

type memberModel struct {
	CardNumber string `gorm:"column:card_number;primaryKey"`
	FullName   string `gorm:"column:full_name"`
	Status     string `gorm:"column:status"`
}

func (memberModel) TableName() string { return "members" }

type ruleModel struct {
	ID         int                `gorm:"column:id;primaryKey"`
	Code       string             `gorm:"column:code"`
	Type       string             `gorm:"column:type"`
	PercentBP  int                `gorm:"column:percent_bp"`
	BundleSize *int               `gorm:"column:bundle_size"`
	Priority   int                `gorm:"column:priority"`
	Active     bool               `gorm:"column:active"`
	Products   []ruleProductModel `gorm:"foreignKey:RuleID"`
}

func (ruleModel) TableName() string { return "rules" }

type ruleProductModel struct {
	RuleID    int    `gorm:"column:rule_id;primaryKey"`
	ProductID string `gorm:"column:product_id;primaryKey"`
}

func (ruleProductModel) TableName() string { return "rule_products" }

type productLimitModel struct {
	ProductID     string `gorm:"column:product_id;primaryKey"`
	WindowSeconds int    `gorm:"column:window_seconds"`
	Active        bool   `gorm:"column:active"`
}

func (productLimitModel) TableName() string { return "product_limits" }

type orderModel struct {
	ID                   string               `gorm:"column:id;primaryKey"`
	MemberCard           *string              `gorm:"column:member_card"`
	TotalBeforeSatang    int64                `gorm:"column:total_before_satang"`
	MemberDiscountSatang int64                `gorm:"column:member_discount_satang"`
	FinalTotalSatang     int64                `gorm:"column:final_total_satang"`
	CreatedAt            time.Time            `gorm:"column:created_at;autoCreateTime:false"`
	Items                []orderItemModel     `gorm:"foreignKey:OrderID"`
	Discounts            []orderDiscountModel `gorm:"foreignKey:OrderID"`
}

func (orderModel) TableName() string { return "orders" }

type orderItemModel struct {
	OrderID   string `gorm:"column:order_id;primaryKey"`
	ProductID string `gorm:"column:product_id;primaryKey"`
	Quantity  int    `gorm:"column:quantity"`
}

func (orderItemModel) TableName() string { return "order_items" }

type orderDiscountModel struct {
	ID           int     `gorm:"column:id;primaryKey;autoIncrement"`
	OrderID      string  `gorm:"column:order_id"`
	RuleCode     string  `gorm:"column:rule_code"`
	ProductID    *string `gorm:"column:product_id"`
	AmountSatang int64   `gorm:"column:amount_satang"`
}

func (orderDiscountModel) TableName() string { return "order_discounts" }
