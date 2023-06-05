package transforms

import (
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"sort"
	"time"
)

func WithPercentile(percentile int) func(durations []time.Duration) time.Duration {
	return func(durations []time.Duration) time.Duration {
		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})
		return durations[int(float64(len(durations))*(float64(percentile)/float64(100)))]
	}
}

func NinetyPercentile(upperBound time.Duration) types.GomegaMatcher {
	return gomega.WithTransform(WithPercentile(90), gomega.BeNumerically("<", upperBound))
}

func Median(upperBound time.Duration) types.GomegaMatcher {
	return gomega.WithTransform(WithPercentile(50), gomega.BeNumerically("<", upperBound))
}

func exampleTest() {
	var durations []time.Duration

	type entry struct {
		name      string
		frequency int

		benchmarkAssertions []types.GomegaMatcher
	}

	var entries = []entry{
		{
			name:      "100",
			frequency: 100,
			benchmarkAssertions: []types.GomegaMatcher{
				Median(time.Millisecond * 10),
				NinetyPercentile(time.Millisecond * 100),
			},
		},
		{
			name:      "1000",
			frequency: 1000,
			benchmarkAssertions: []types.GomegaMatcher{
				Median(time.Millisecond * 100),
				NinetyPercentile(time.Millisecond * 1000),
				gomega.WithTransform(WithPercentile(30), gomega.BeNumerically("<", time.Second)),
			},
		},
	}

	for _, entry := range entries {
		gomega.Expect(durations).Should(gomega.And(entry.benchmarkAssertions...))
	}
}
