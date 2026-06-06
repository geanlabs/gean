package forkchoice

const maxScore = int64(1<<63 - 1)

func quorumScore(numValidators uint64) int64 {
	q := numValidators / 3
	r := numValidators % 3

	score := 2 * q
	if r > 0 {
		score += r
	}
	if score > uint64(maxScore) {
		return maxScore
	}
	return int64(score)
}
