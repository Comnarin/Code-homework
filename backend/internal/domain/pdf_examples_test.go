package domain

import "testing"

// Every example and condition in "Code Homework Test.pdf" as an executable test.
func TestCalculate_PDFExamples(t *testing.T) {
	pair, err := NewBundlePercent("PAIR_5", 10, []ProductID{"orange", "pink", "green"}, 2, 500)
	if err != nil {
		t.Fatal(err)
	}
	member, err := NewMemberPercent("MEMBER_10", 100, 1000)
	if err != nil {
		t.Fatal(err)
	}
	rules := []Rule{pair, member}

	tests := []struct {
		name       string
		lines      []Line
		isMember   bool
		wantBefore Money
		wantMember Money
		wantFinal  Money
	}{
		{"Orange x2 = (120+120) - 5%", []Line{{orange, 2}}, false, 24000, 0, 22800},
		{"Pink x4 = two discounted pairs", []Line{{pink, 4}}, false, 32000, 0, 30400},
		{"Green x3 = one discounted pair + 40", []Line{{green, 3}}, false, 12000, 0, 11600},
		{"Blue gets no pair discount", []Line{{blue, 2}}, false, 6000, 0, 6000},
		{"member gets 10% of the total", []Line{{blue, 2}}, true, 6000, 600, 5400},
		{"member 10% applies after pair discount", []Line{{orange, 2}}, true, 24000, 2280, 20520},
		{"mixed order", []Line{{orange, 2}, {green, 3}, {blue, 1}}, false, 24000 + 12000 + 3000, 0, 39000 - 1200 - 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Calculate(tt.lines, tt.isMember, rules)
			if err != nil {
				t.Fatal(err)
			}
			if q.TotalBefore != tt.wantBefore || q.MemberDiscount() != tt.wantMember || q.FinalTotal != tt.wantFinal {
				t.Fatalf("got before=%d member=%d final=%d; want %d/%d/%d",
					q.TotalBefore, q.MemberDiscount(), q.FinalTotal, tt.wantBefore, tt.wantMember, tt.wantFinal)
			}
		})
	}
}
