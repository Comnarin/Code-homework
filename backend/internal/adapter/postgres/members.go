package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
)

type MemberRepository struct{ db *gorm.DB }

func NewMemberRepository(db *gorm.DB) *MemberRepository { return &MemberRepository{db: db} }

func (r *MemberRepository) FindByCard(ctx context.Context, card string) (domain.Member, bool, error) {
	var rows []memberModel
	// Find+Limit instead of First: a missing card is a normal outcome, not an error to log.
	if err := r.db.WithContext(ctx).Where("card_number = ?", card).Limit(1).Find(&rows).Error; err != nil {
		return domain.Member{}, false, fmt.Errorf("query member: %w", err)
	}
	if len(rows) == 0 {
		return domain.Member{}, false, nil
	}
	m := rows[0]
	return domain.Member{CardNumber: m.CardNumber, FullName: m.FullName, Active: m.Status == "ACTIVE"}, true, nil
}
