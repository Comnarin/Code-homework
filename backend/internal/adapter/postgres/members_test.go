package postgres

import (
	"context"
	"testing"

	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.MemberRepository = (*MemberRepository)(nil)

func TestMemberRepository_FindByCard(t *testing.T) {
	repo := NewMemberRepository(testPool(t))
	ctx := context.Background()

	m, found, err := repo.FindByCard(ctx, "MEM0001")
	if err != nil || !found || !m.Active || m.CardNumber != "MEM0001" {
		t.Fatalf("active member: %+v found=%v err=%v", m, found, err)
	}
	m, found, err = repo.FindByCard(ctx, "MEM0002")
	if err != nil || !found || m.Active {
		t.Fatalf("suspended member should be found but inactive: %+v found=%v err=%v", m, found, err)
	}
	if _, found, err = repo.FindByCard(ctx, "NOPE"); err != nil || found {
		t.Fatalf("unknown card: found=%v err=%v, want found=false err=nil", found, err)
	}
}
