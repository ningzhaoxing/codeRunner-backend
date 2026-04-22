package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"go.uber.org/zap"
)

// StartPusher 周期性把默认 registry 的指标推到 pushgateway。
// 返回 stop 函数；url 为空时返回 nil。
func StartPusher(url, jobName, instance string, interval time.Duration) func() {
	if url == "" {
		zap.S().Info("metrics pushgateway disabled (empty url)")
		return nil
	}
	if jobName == "" {
		jobName = "code-runner-worker"
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if instance == "" {
		instance = "unknown"
	}

	pusher := push.New(url, jobName).
		Gatherer(prometheus.DefaultGatherer).
		Grouping("instance", instance)

	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		zap.S().Infof("metrics pusher started: url=%s job=%s instance=%s interval=%s", url, jobName, instance, interval)
		for {
			select {
			case <-ticker.C:
				if err := pusher.Push(); err != nil {
					zap.S().Warnf("metrics push failed: %v", err)
				}
			case <-stopCh:
				// 退出前最后 push 一次
				if err := pusher.Push(); err != nil {
					zap.S().Warnf("metrics final push failed: %v", err)
				}
				zap.S().Info("metrics pusher stopped")
				return
			}
		}
	}()

	return func() { close(stopCh) }
}
