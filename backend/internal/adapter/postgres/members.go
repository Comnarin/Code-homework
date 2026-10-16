package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type MemberRepository struct{ pool *pgxpool.Pool }

func NewMemberRepository(pool *pgxpool.Pool) *MemberRepository { return &MemberRepository{pool: pool} }

func (r *MemberRepository) FindByCard(ctx context.Context, card string) (domain.Member, bool, error) {
	var m domain.Member
	var status string
	err := r.pool.QueryRow(ctx, "SELECT card_number, full_name, status FROM members WHERE card_number = $1", card).
		Scan(&m.CardNumber, &m.FullName, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Member{}, false, nil
	}
	if err != nil {
		return domain.Member{}, false, fmt.Errorf("query member: %w", err)
	}
	m.Active = status == "ACTIVE"
	return m, true, nil
}
