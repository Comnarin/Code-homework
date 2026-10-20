package httpapi

import "github.com/comnarin/foodstore/internal/domain"

type productDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PriceSatang int64  `json:"priceSatang"`
	// ImageUrl always points at the image endpoint, which serves the default
	// picture when the product has none. The storage key is never exposed.
	ImageURL string `json:"imageUrl"`
}

type promotionDTO struct {
	Code       string   `json:"code"`
	Type       string   `json:"type"`
	Percent    int      `json:"percent"`
	BundleSize int      `json:"bundleSize,omitempty"`
	ProductIDs []string `json:"productIds,omitempty"`
}

type calculateRequest struct {
	MemberCard string `json:"memberCard"`
	Items      []struct {
		ProductID string `json:"productId"`
		Quantity  int    `json:"quantity"`
	} `json:"items"`
}

type itemDiscountDTO struct {
	RuleCode     string `json:"ruleCode"`
	ProductID    string `json:"productId"`
	Name         string `json:"name"`
	Bundles      int    `json:"bundles"`
	AmountSatang int64  `json:"amountSatang"`
}

type orderItemDTO struct {
	ProductID       string `json:"productId"`
	Name            string `json:"name"`
	Quantity        int    `json:"quantity"`
	UnitPriceSatang int64  `json:"unitPriceSatang"`
	LineTotalSatang int64  `json:"lineTotalSatang"`
}

type calculateResponse struct {
	Items                []orderItemDTO    `json:"items"`
	TotalBeforeSatang    int64             `json:"totalBeforeSatang"`
	ItemDiscounts        []itemDiscountDTO `json:"itemDiscounts"`
	MemberDiscountSatang int64             `json:"memberDiscountSatang"`
	MemberBaseSatang     int64             `json:"memberBaseSatang"`
	FinalTotalSatang     int64             `json:"finalTotalSatang"`
}

func toProductDTOs(items []domain.Product) []productDTO {
	out := make([]productDTO, 0, len(items))
	for _, p := range items {
		out = append(out, productDTO{
			ID: string(p.ID), Name: p.Name, PriceSatang: int64(p.Price),
			ImageURL: "/api/products/" + string(p.ID) + "/image",
		})
	}
	return out
}

func toPromotionDTOs(items []domain.Promotion) []promotionDTO {
	out := make([]promotionDTO, 0, len(items))
	for _, p := range items {
		ids := make([]string, len(p.ProductIDs))
		for i, id := range p.ProductIDs {
			ids[i] = string(id)
		}
		out = append(out, promotionDTO{
			Code: p.Code, Type: string(p.Kind), Percent: p.Percent,
			BundleSize: p.BundleSize, ProductIDs: ids,
		})
	}
	return out
}

func toCalculateResponse(q domain.Quote) calculateResponse {
	items := make([]itemDiscountDTO, 0, len(q.Discounts))
	for _, d := range q.ItemDiscounts() {
		items = append(items, itemDiscountDTO{
			RuleCode: d.RuleCode, ProductID: string(d.ProductID), Name: d.Name,
			Bundles: d.Bundles, AmountSatang: int64(d.Amount),
		})
	}
	lines := make([]orderItemDTO, 0, len(q.Lines))
	for _, l := range q.Lines {
		lines = append(lines, orderItemDTO{
			ProductID:       string(l.Product.ID),
			Name:            l.Product.Name,
			Quantity:        l.Quantity,
			UnitPriceSatang: int64(l.Product.Price),
			LineTotalSatang: int64(l.Product.Price) * int64(l.Quantity),
		})
	}
	return calculateResponse{
		Items:                lines,
		TotalBeforeSatang:    int64(q.TotalBefore),
		ItemDiscounts:        items,
		MemberDiscountSatang: int64(q.MemberDiscount()),
		MemberBaseSatang:     int64(q.MemberDiscountBase()),
		FinalTotalSatang:     int64(q.FinalTotal),
	}
}
