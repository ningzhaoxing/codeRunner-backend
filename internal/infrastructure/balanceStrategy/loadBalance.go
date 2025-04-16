package balanceStrategy

import "codeRunner-siwu/internal/infrastructure/balanceStrategy/weightedRRBalance"

type LoadBalance interface {
	Add(*weightedRRBalance.WeightNode)
	Get() (*weightedRRBalance.WeightNode, error)
	Remove(string)
}
