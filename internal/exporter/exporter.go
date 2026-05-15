// Prometheus text-format exporter for astral-spy.
//
// Hand-rolled (no client_golang dep) — the metric set is small and stable.
// Each scrape re-reads NVML + i2c, so we never serve a stale value.

package exporter

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/villainsquad/astral-spy/internal/sus"
)

type Server struct {
	addr    string
	devices []sus.AstralDevice
	mu      sync.Mutex // serialise scrapes; smbus reads aren't reentrant
}

func New(addr string, devices []sus.AstralDevice) *Server {
	return &Server{addr: addr, devices: devices}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "astral-spy exporter — see /metrics")
	})
	srv := &http.Server{Addr: s.addr, Handler: mux}
	return srv.ListenAndServe()
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.collect(w)
}

func (s *Server) collect(w io.Writer) {
	// Per-device pin metrics
	writeHelp(w, "astral_pin_watts", "gauge", "Per-pin power draw on the 12V-2x6 connector (W)")
	writeHelp(w, "astral_pin_volts", "gauge", "Per-pin voltage (V)")
	writeHelp(w, "astral_pin_amps", "gauge", "Per-pin current (A)")
	writeHelp(w, "astral_pin_total_watts", "gauge", "Sum of all pins on the connector (W)")
	writeHelp(w, "astral_pin_balance_ratio", "gauge", "min(pin) / max(pin) — 1.0 is perfectly even")

	// NVML metrics (mirrors the TUI)
	writeHelp(w, "astral_gpu_temp_celsius", "gauge", "GPU core temperature (°C)")
	writeHelp(w, "astral_gpu_power_watts", "gauge", "GPU board power draw (W)")
	writeHelp(w, "astral_gpu_power_limit_watts", "gauge", "GPU power management limit (W)")
	writeHelp(w, "astral_gpu_util_percent", "gauge", "GPU compute utilisation (0–100)")
	writeHelp(w, "astral_gpu_mem_util_percent", "gauge", "GPU memory controller utilisation (0–100)")
	writeHelp(w, "astral_gpu_fan_percent", "gauge", "Fan speed (0–100)")
	writeHelp(w, "astral_gpu_clock_graphics_mhz", "gauge", "Core clock (MHz)")
	writeHelp(w, "astral_gpu_clock_memory_mhz", "gauge", "Memory clock (MHz)")
	writeHelp(w, "astral_gpu_memory_used_bytes", "gauge", "VRAM in use (bytes)")
	writeHelp(w, "astral_gpu_memory_total_bytes", "gauge", "VRAM total (bytes)")
	writeHelp(w, "astral_gpu_up", "gauge", "1 if the device responded to this scrape, 0 if any read failed")

	for i, dev := range s.devices {
		gpuIdx := strconv.Itoa(i)
		uuid := dev.Identifier()

		metrics, mErr := sus.ReadAstralDeviceMetrics(dev)
		pins, pErr := sus.ReadAstralDevicePins(dev)

		labels := map[string]string{
			"gpu":      gpuIdx,
			"gpu_uuid": uuid,
			"gpu_name": metrics.Name,
		}

		up := 1.0
		if mErr != nil || pErr != nil {
			up = 0
		}
		writeSample(w, "astral_gpu_up", labels, up)

		if mErr == nil {
			writeSample(w, "astral_gpu_temp_celsius", labels, float64(metrics.TempC))
			writeSample(w, "astral_gpu_power_watts", labels, metrics.PowerUsageW)
			writeSample(w, "astral_gpu_power_limit_watts", labels, metrics.PowerLimitW)
			writeSample(w, "astral_gpu_util_percent", labels, float64(metrics.UtilGpu))
			writeSample(w, "astral_gpu_mem_util_percent", labels, float64(metrics.UtilMem))
			writeSample(w, "astral_gpu_fan_percent", labels, float64(metrics.FanPercent))
			writeSample(w, "astral_gpu_clock_graphics_mhz", labels, float64(metrics.ClockGraphicsMHz))
			writeSample(w, "astral_gpu_clock_memory_mhz", labels, float64(metrics.ClockMemoryMHz))
			writeSample(w, "astral_gpu_memory_used_bytes", labels, float64(metrics.MemoryUsedBytes))
			writeSample(w, "astral_gpu_memory_total_bytes", labels, float64(metrics.MemoryTotalBytes))
		}

		if pErr != nil {
			// astral_gpu_up was already set to 0 above; that's the alertable signal.
			continue
		}

		total, upper, lower := 0.0, 0.0, 1e9
		for _, p := range pins {
			v := p.Drawing()
			total += v
			if v > upper {
				upper = v
			}
			if v < lower {
				lower = v
			}
		}
		balance := 1.0
		if upper > 0 {
			balance = lower / upper
		}

		writeSample(w, "astral_pin_total_watts", labels, total)
		writeSample(w, "astral_pin_balance_ratio", labels, balance)

		for j, p := range pins {
			pinLabels := mergeLabels(labels, map[string]string{"pin": strconv.Itoa(j + 1)})
			writeSample(w, "astral_pin_watts", pinLabels, p.Drawing())
			writeSample(w, "astral_pin_volts", pinLabels, p.Voltage())
			writeSample(w, "astral_pin_amps", pinLabels, p.Current())
		}
	}
}

func writeHelp(w io.Writer, name, kind, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

func writeSample(w io.Writer, name string, labels map[string]string, value float64) {
	fmt.Fprintf(w, "%s%s %s\n", name, formatLabels(labels), strconv.FormatFloat(value, 'g', -1, 64))
}

// Prometheus label values must be quoted and have \, ", \n escaped.
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	// Stable key order for cache-friendliness and easier diffing in tests.
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sortStrings(keys)

	var b []byte
	b = append(b, '{')
	for i, k := range keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, k...)
		b = append(b, '=', '"')
		b = appendEscaped(b, labels[k])
		b = append(b, '"')
	}
	b = append(b, '}')
	return string(b)
}

func appendEscaped(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			dst = append(dst, '\\', '\\')
		case '"':
			dst = append(dst, '\\', '"')
		case '\n':
			dst = append(dst, '\\', 'n')
		default:
			dst = append(dst, c)
		}
	}
	return dst
}

func mergeLabels(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// Tiny insertion sort to avoid pulling sort just for label keys.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
