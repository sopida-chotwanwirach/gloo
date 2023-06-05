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

func exampleTest() {
    var durations []time.Duration

    gomega.Expect(durations).To(NinetyPercentile(time.Millisecond * 100))
}



