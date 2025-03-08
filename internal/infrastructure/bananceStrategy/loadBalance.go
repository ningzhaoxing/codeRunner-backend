package bananceStrategy

import "codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"

type LoadBalance interface {
	Add(*weightedRRBalance.WeightNode)
	Get() (*weightedRRBalance.WeightNode, error)
	Remove(string)
	UpdateWeight(string, int64)
}
