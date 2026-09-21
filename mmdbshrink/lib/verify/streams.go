package verify

import (
	"fmt"
	"iter"

	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/format"
	"github.com/oschwald/maxminddb-golang/v2"
)

// compareExactPrefixStreams walks both files' prefix sets in lockstep and
// reports the first position where they diverge. Lookup equivalence does not
// require identical layouts, which is why this is a separate, stricter check.
func compareExactPrefixStreams(
	a, b *maxminddb.Reader,
	opts []maxminddb.NetworksOption,
	limit int,
) (int, int, bool, string, error) {
	nextA, stopA := iter.Pull(a.NetworksWithin(rootPrefix(a), opts...))
	defer stopA()
	nextB, stopB := iter.Pull(b.NetworksWithin(rootPrefix(b), opts...))
	defer stopB()

	countA := 0
	countB := 0
	for {
		var resultA, resultB maxminddb.Result
		var okA, okB bool
		if limit <= 0 || countA < limit {
			resultA, okA = nextA()
		}
		if limit <= 0 || countB < limit {
			resultB, okB = nextB()
		}
		if !okA && !okB {
			return countA, countB, true, "", nil
		}
		if okA {
			if err := resultA.Err(); err != nil {
				return countA, countB, false, "", err
			}
			countA++
		}
		if okB {
			if err := resultB.Err(); err != nil {
				return countA, countB, false, "", err
			}
			countB++
		}
		if okA != okB {
			return countA, countB, false,
				fmt.Sprintf("prefix count differs: %d vs %d", countA, countB), nil
		}
		pfxA := resultA.Prefix().Masked()
		pfxB := resultB.Prefix().Masked()
		if pfxA != pfxB {
			return countA, countB, false,
				fmt.Sprintf("prefix differs at position %s: baseline %s vs shrunk %s",
					format.Int(uint64(countA)), pfxA, pfxB), nil
		}
	}
}
