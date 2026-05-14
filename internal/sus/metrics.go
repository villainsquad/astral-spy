// NVML accessors used by the astral-spy terminal dashboard.

package sus

import "fmt"

import "github.com/NVIDIA/go-nvml/pkg/nvml"

type AstralDeviceMetrics struct {
	Name             string
	UtilGpu          uint32
	UtilMem          uint32
	TempC            uint32
	FanPercent       uint32
	ClockGraphicsMHz uint32
	ClockMemoryMHz   uint32
	PowerUsageW      float64
	PowerLimitW      float64
	MemoryTotalBytes uint64
	MemoryUsedBytes  uint64
}

func ReadAstralDeviceMetrics(target AstralDevice) (AstralDeviceMetrics, error) {
	var m AstralDeviceMetrics

	name, ret := nvml.DeviceGetName(target.deviceHandle)
	if ret != nvml.SUCCESS {
		return m, fmt.Errorf("nvmlDeviceGetName failed")
	}
	m.Name = name

	util, ret := nvml.DeviceGetUtilizationRates(target.deviceHandle)
	if ret == nvml.SUCCESS {
		m.UtilGpu = util.Gpu
		m.UtilMem = util.Memory
	}

	temp, ret := nvml.DeviceGetTemperature(target.deviceHandle, nvml.TEMPERATURE_GPU)
	if ret == nvml.SUCCESS {
		m.TempC = temp
	}

	fan, ret := nvml.DeviceGetFanSpeed(target.deviceHandle)
	if ret == nvml.SUCCESS {
		m.FanPercent = fan
	}

	clkGfx, ret := nvml.DeviceGetClockInfo(target.deviceHandle, nvml.CLOCK_GRAPHICS)
	if ret == nvml.SUCCESS {
		m.ClockGraphicsMHz = clkGfx
	}

	clkMem, ret := nvml.DeviceGetClockInfo(target.deviceHandle, nvml.CLOCK_MEM)
	if ret == nvml.SUCCESS {
		m.ClockMemoryMHz = clkMem
	}

	pUse, ret := nvml.DeviceGetPowerUsage(target.deviceHandle)
	if ret == nvml.SUCCESS {
		m.PowerUsageW = float64(pUse) / 1000
	}

	pLim, ret := nvml.DeviceGetPowerManagementLimit(target.deviceHandle)
	if ret == nvml.SUCCESS {
		m.PowerLimitW = float64(pLim) / 1000
	}

	mem, ret := nvml.DeviceGetMemoryInfo(target.deviceHandle)
	if ret == nvml.SUCCESS {
		m.MemoryTotalBytes = mem.Total
		m.MemoryUsedBytes = mem.Used
	}

	return m, nil
}
