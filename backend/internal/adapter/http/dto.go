package httpapi

import "github.com/comnarin/foodstore/internal/domain"

type productDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PriceSatang int64  `json:"priceSatang"`
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

type calculateResponse struct {
	TotalBeforeSatang    int64             `json:"totalBeforeSatang"`
	ItemDiscounts        []itemDiscountDTO `json:"itemDiscounts"`
	MemberDiscountSatang int64             `json:"memberDiscountSatang"`
	FinalTotalSatang     int64             `json:"finalTotalSatang"`
}

func toProductDTOs(items []domain.Product) []productDTO {
	out := make([]productDTO, 0, len(items))
	for _, p := range items {
		out = append(out, productDTO{ID: string(p.ID), Name: p.Name, PriceSatang: int64(p.Price)})
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
	return calculateResponse{
		TotalBeforeSatang:    int64(q.TotalBefore),
		ItemDiscounts:        items,
		MemberDiscountSatang: int64(q.MemberDiscount()),
		FinalTotalSatang:     int64(q.FinalTotal),
	}
}
