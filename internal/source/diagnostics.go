package source

import (
	"net/http"
	"sync"
	"time"
)

type NetworkMetric struct {
	Host          string `json:"host"`
	Requests      int64  `json:"requests"`
	Failures      int64  `json:"failures"`
	LastStatus    int    `json:"lastStatus"`
	LastErrorKind string `json:"lastErrorKind"`
	AverageMS     int64  `json:"averageMs"`
	ElapsedMS     int64  `json:"-"`
}

var networkMetrics = struct {
	sync.Mutex
	hosts map[string]NetworkMetric
}{hosts: map[string]NetworkMetric{}}

type observedTransport struct{ base http.RoundTripper }

func (t observedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(r)
	networkMetrics.Lock()
	defer networkMetrics.Unlock()
	host := r.URL.Hostname()
	m := networkMetrics.hosts[host]
	m.Host = host
	m.Requests++
	m.ElapsedMS += time.Since(start).Milliseconds()
	m.AverageMS = m.ElapsedMS / m.Requests
	if err != nil {
		m.Failures++
		m.LastStatus = 0
		m.LastErrorKind = ErrorKind(err)
	} else {
		m.LastStatus = resp.StatusCode
		m.LastErrorKind = ""
		if resp.StatusCode >= 400 {
			m.Failures++
			m.LastErrorKind = httpFailure("", resp.StatusCode, resp.Header).Category
		}
	}
	if len(networkMetrics.hosts) > 64 {
		networkMetrics.hosts = map[string]NetworkMetric{}
	}
	networkMetrics.hosts[host] = m
	return resp, err
}
func NetworkStats() []NetworkMetric {
	networkMetrics.Lock()
	defer networkMetrics.Unlock()
	out := []NetworkMetric{}
	for _, m := range networkMetrics.hosts {
		out = append(out, m)
	}
	return out
}
