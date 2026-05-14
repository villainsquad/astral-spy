// astral-spy — a compact terminal dashboard for ASUS ROG Astral RTX 5090
// power-pin monitoring combined with nvidia-smi style metrics.

package main

import "flag"
import "fmt"
import "io"
import "os"
import "os/signal"
import "strings"
import "syscall"
import "time"

import "github.com/NVIDIA/go-nvml/pkg/nvml"
import "github.com/villainsquad/astral-spy/internal/sus"

// ANSI control sequences (vars, not consts, so --no-color can clear them).
var (
	ansiReset    = "\033[0m"
	ansiBold     = "\033[1m"
	ansiDim      = "\033[2m"
	ansiRed      = "\033[31m"
	ansiGreen    = "\033[32m"
	ansiYellow   = "\033[33m"
	ansiHide     = "\033[?25l"
	ansiShow     = "\033[?25h"
	ansiClear    = "\033[2J"
	ansiHome     = "\033[H"
	ansiClearEOL = "\033[K"
)

// Thresholds tuned for ASUS ROG Astral RTX 5090 (575–600 W TGP, +20% slider
// up to ~690 W) on a 12V-2x6 connector (per-pin Molex MicroFit 3.0, ~9.5 A
// rated → ~114 W/pin). At rated 600 W evenly balanced, ideal per-pin draw
// is 100 W, which sets the warn floor.
//
//   * pinWarnW / pinCritW: per-pin power draw thresholds. Crit matches
//     susd's default emergency-throttle line so dashboard red = daemon act.
//   * balanceWarnPct / balanceCritPct: lower/upper pin draw ratio.
//     Healthy 5090s measure ~0.93 under load, so warn=0.85 leaves
//     headroom while crit=0.70 stays clear for real faults.
//   * tempWarnC / tempCritC: Blackwell throttles ~88 °C.
var (
	pinWarnW       = 100.0
	pinCritW       = 105.0
	balanceWarnPct = 0.85
	balanceCritPct = 0.70
	// Below this per-pin draw the balance ratio is dominated by ADC
	// noise (sub-amp shunt readings), so imbalance status is suppressed.
	balanceFloorW = 10.0
	tempWarnC     = uint32(80)
	tempCritC     = uint32(87)
)

func main() {
	interval := flag.Duration("t", time.Second, "Refresh interval")
	noColor := flag.Bool("no-color", false, "Disable ANSI colors")
	once := flag.Bool("once", false, "Print one frame and exit (no clear screen)")
	flag.Float64Var(&pinWarnW, "pin-warn", pinWarnW, "Per-pin warning threshold (W)")
	flag.Float64Var(&pinCritW, "pin-crit", pinCritW, "Per-pin critical threshold (W)")
	flag.Float64Var(&balanceFloorW, "bal-floor", balanceFloorW,
		"Suppress imbalance status when min pin draw is below this (W)")
	flag.Parse()

	if *noColor {
		disableColors()
	}

	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		fmt.Fprintln(os.Stderr, "nvmlInit failed (is the NVIDIA driver loaded?)")
		os.Exit(1)
	}
	defer nvml.Shutdown()

	devices, err := sus.FindAstralDevices()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(devices) < 1 {
		fmt.Fprintln(os.Stderr, "No compatible ASUS ROG Astral RTX 5090 devices found.")
		os.Exit(1)
	}

	out := os.Stdout

	if *once {
		render(out, devices, *interval)
		return
	}

	// Catch interrupt so we can restore the cursor cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprint(out, ansiShow)
		fmt.Fprint(out, "\n")
		os.Exit(0)
	}()

	fmt.Fprint(out, ansiHide, ansiClear)
	defer fmt.Fprint(out, ansiShow)

	for {
		fmt.Fprint(out, ansiHome)
		render(out, devices, *interval)
		time.Sleep(*interval)
	}
}

func render(out io.Writer, devices []sus.AstralDevice, interval time.Duration) {
	now := time.Now().Format("15:04:05")
	header := fmt.Sprintf("%sNVIDIA Astral Power Pin Monitor%s", ansiBold, ansiReset)
	right := fmt.Sprintf("%s%s%s", ansiDim, now, ansiReset)
	writeRow(out, header, right, 78)
	writeRule(out, 78)

	for i, dev := range devices {
		renderDevice(out, i, dev)
	}

	footer := fmt.Sprintf("%srefresh %s · Ctrl-C to quit%s",
		ansiDim, interval, ansiReset)
	fmt.Fprintln(out, footer+ansiClearEOL)
	// Clear from cursor to end of screen so a previous (longer) frame
	// does not leave stale lines below.
	fmt.Fprint(out, "\033[J")
}

func renderDevice(out io.Writer, index int, device sus.AstralDevice) {
	metrics, mErr := sus.ReadAstralDeviceMetrics(device)
	pins, pErr := sus.ReadAstralDevicePins(device)

	name := metrics.Name
	if name == "" {
		name = "(unknown)"
	}
	fmt.Fprintf(out, "%sGPU %d%s  %s  %s%s%s\n%s",
		ansiBold, index, ansiReset, name,
		ansiDim, device.Identifier(), ansiReset, ansiClearEOL)

	if mErr != nil {
		fmt.Fprintf(out, "  %smetrics error: %v%s\n%s",
			ansiRed, mErr, ansiReset, ansiClearEOL)
	} else {
		writeMetrics(out, metrics)
	}

	fmt.Fprint(out, ansiClearEOL+"\n")

	if pErr != nil {
		fmt.Fprintf(out, "  %spin read error: %v%s\n%s",
			ansiRed, pErr, ansiReset, ansiClearEOL)
		fmt.Fprint(out, ansiClearEOL+"\n")
		return
	}

	writePins(out, pins)
	fmt.Fprint(out, ansiClearEOL+"\n")
}

func writeMetrics(out io.Writer, m sus.AstralDeviceMetrics) {
	memUsedGiB := float64(m.MemoryUsedBytes) / (1024 * 1024 * 1024)
	memTotalGiB := float64(m.MemoryTotalBytes) / (1024 * 1024 * 1024)
	memPct := 0.0
	if m.MemoryTotalBytes > 0 {
		memPct = float64(m.MemoryUsedBytes) / float64(m.MemoryTotalBytes) * 100
	}

	tempStr := fmt.Sprintf("%d°C", m.TempC)
	switch {
	case m.TempC >= tempCritC:
		tempStr = paint(ansiRed, tempStr)
	case m.TempC >= tempWarnC:
		tempStr = paint(ansiYellow, tempStr)
	default:
		tempStr = paint(ansiGreen, tempStr)
	}

	powerStr := fmt.Sprintf("%.1f / %.0f W", m.PowerUsageW, m.PowerLimitW)
	if m.PowerLimitW > 0 && m.PowerUsageW/m.PowerLimitW > 0.95 {
		powerStr = paint(ansiRed, powerStr)
	}

	utilStr := paintByPct(fmt.Sprintf("%3d%%", m.UtilGpu), float64(m.UtilGpu)/100)
	memUtilStr := paintByPct(fmt.Sprintf("%3d%%", m.UtilMem), float64(m.UtilMem)/100)
	fanStr := fmt.Sprintf("%3d%%", m.FanPercent)

	fmt.Fprintf(out, "  Util %s  Mem %s  Temp %s  Fan %s  %s%s\n%s",
		utilStr, memUtilStr, tempStr, fanStr, powerStr, ansiClearEOL, "")
	fmt.Fprintf(out, "  Core %4d MHz   Mem %4d MHz   VRAM %5.1f / %5.1f GiB (%.0f%%)%s\n%s",
		m.ClockGraphicsMHz, m.ClockMemoryMHz, memUsedGiB, memTotalGiB, memPct, ansiClearEOL, "")
}

func writePins(out io.Writer, pins []sus.AstralDevicePin) {
	total := 0.0
	upper := 0.0
	lower := 1e9
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

	status, statusColor := pinStatus(upper, lower, balance)
	statusStr := paint(statusColor, "● "+status)

	fmt.Fprintf(out, "  %s12V-2x6 Pin Distribution%s   Σ %6.1f W   Bal %5.1f%%   %s%s\n%s",
		ansiBold, ansiReset, total, balance*100, statusStr, ansiClearEOL, "")

	// Scale bars to per-pin critical threshold so visual fill maps to a
	// known safety margin rather than a free-floating max.
	scale := pinCritW
	for i, p := range pins {
		drawW := p.Drawing()
		bar := makeBar(drawW, scale, 22)

		col := ansiGreen
		switch {
		case drawW >= pinCritW:
			col = ansiRed
		case drawW >= pinWarnW:
			col = ansiYellow
		}

		fmt.Fprintf(out, "  Pin %d  %s%6.1f W%s  %s%s%s  %5.2f V  %5.2f A%s\n%s",
			i+1, col, drawW, ansiReset, col, bar, ansiReset,
			p.Voltage(), p.Current(), ansiClearEOL, "")
	}
}

func pinStatus(upperW, lowerW, balance float64) (string, string) {
	// Overload always wins regardless of load level.
	if upperW >= pinCritW {
		return "OVERLOAD", ansiRed
	}
	// At low load the balance ratio is noise-dominated (shunt ADC offset
	// swamps sub-amp pin readings). Mirror susd's lowerDraw > 10 W gate:
	// only flag imbalance once every pin is carrying real current.
	if lowerW < balanceFloorW {
		return "IDLE", ansiGreen
	}
	switch {
	case balance < balanceCritPct:
		return "IMBALANCE", ansiRed
	case upperW >= pinWarnW:
		return "ELEVATED", ansiYellow
	case balance < balanceWarnPct:
		return "UNEVEN", ansiYellow
	default:
		return "NORMAL", ansiGreen
	}
}

func makeBar(value, scale float64, width int) string {
	if scale <= 0 {
		scale = 1
	}
	frac := value / scale
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac * float64(width))
	return strings.Repeat("█", filled) + strings.Repeat("·", width-filled)
}

func paint(color, s string) string {
	return color + s + ansiReset
}

func paintByPct(s string, frac float64) string {
	switch {
	case frac >= 0.95:
		return paint(ansiRed, s)
	case frac >= 0.80:
		return paint(ansiYellow, s)
	default:
		return paint(ansiGreen, s)
	}
}

// writeRow prints a left/right justified row to the given visible width.
// ANSI escape sequences are stripped when measuring width.
func writeRow(out io.Writer, left, right string, width int) {
	leftLen := visibleLen(left)
	rightLen := visibleLen(right)
	pad := width - leftLen - rightLen
	if pad < 1 {
		pad = 1
	}
	fmt.Fprintf(out, "%s%s%s%s\n", left, strings.Repeat(" ", pad), right, ansiClearEOL)
}

func writeRule(out io.Writer, width int) {
	fmt.Fprintf(out, "%s%s%s%s\n", ansiDim, strings.Repeat("─", width), ansiReset, ansiClearEOL)
}

func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			continue
		}
		n++
	}
	return n
}

func disableColors() {
	ansiReset = ""
	ansiBold = ""
	ansiDim = ""
	ansiRed = ""
	ansiGreen = ""
	ansiYellow = ""
}
