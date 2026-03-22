// Building intent data type definitions.
// BuildingIntent is the core output of the intent collection phase.
// It stores building design parameters gathered from user descriptions
// and drives YAML configuration file generation.

package intent

// BuildingInfo holds basic building identification and site data.
type BuildingInfo struct {
	Name      string  `json:"name"`       // Building name (English)
	Type      string  `json:"type"`       // Residential / Office / Commercial / Education / Hotel / Industrial
	City      string  `json:"city"`       // City name (used for climate inference)
	Latitude  float64 `json:"latitude"`   // Decimal degrees
	Longitude float64 `json:"longitude"`  // Decimal degrees
	TimeZone  float64 `json:"time_zone"`  // UTC offset (e.g. 8.0 for UTC+8)
	Elevation float64 `json:"elevation"`  // Meters above sea level
	Terrain   string  `json:"terrain"`    // City / Suburbs / Country / Ocean / Desert
	NorthAxis float64 `json:"north_axis"` // Degrees clockwise from north (usually 0)
}

// ZoneLayout describes a single thermal zone.
type ZoneLayout struct {
	Name       string `json:"name"`       // Zone name, e.g. "Zone_F1"
	Floor      int    `json:"floor"`      // Floor number (1-based)
	Multiplier int    `json:"multiplier"` // Zone multiplier (1 = no multiplication)
}

// GeometrySpec describes the building's physical dimensions and zone layout.
type GeometrySpec struct {
	NumFloors   int          `json:"num_floors"`   // Total number of floors
	TotalArea   float64      `json:"total_area"`   // Total floor area (m²)
	FloorWidth  float64      `json:"floor_width"`  // Building width along X axis (m)
	FloorDepth  float64      `json:"floor_depth"`  // Building depth along Y axis (m)
	FloorHeight float64      `json:"floor_height"` // Typical floor-to-floor height (m)
	Zones       []ZoneLayout `json:"zones"`        // One zone per floor for simple buildings
}

// EnvelopeSpec holds thermal performance parameters for the building envelope.
type EnvelopeSpec struct {
	WallU      float64 `json:"wall_u"`     // Exterior wall U-value (W/m²K)
	RoofU      float64 `json:"roof_u"`     // Roof U-value (W/m²K)
	FloorU     float64 `json:"floor_u"`    // Ground floor U-value (W/m²K)
	Insulation string  `json:"insulation"` // Insulation material: EPS / XPS / Rockwool
}

// WindowSpec holds glazing parameters and window-to-wall ratios per facade.
type WindowSpec struct {
	WWRSouth float64 `json:"wwr_south"` // South facade window-to-wall ratio (0–1)
	WWRNorth float64 `json:"wwr_north"` // North facade window-to-wall ratio (0–1)
	WWREast  float64 `json:"wwr_east"`  // East facade window-to-wall ratio (0–1)
	WWRWest  float64 `json:"wwr_west"`  // West facade window-to-wall ratio (0–1)
	UFactor  float64 `json:"u_factor"`  // Window U-value (W/m²K)
	SHGC     float64 `json:"shgc"`      // Solar Heat Gain Coefficient (0–1)
	VT       float64 `json:"vt"`        // Visible transmittance (0–1)
}

// LoadsSpec holds internal heat gain parameters.
type LoadsSpec struct {
	OccupancyType    string  `json:"occupancy_type"`    // residential / office / commercial / education / hotel
	OccupancyDensity float64 `json:"occupancy_density"` // People per m²
	LightingPower    float64 `json:"lighting_power"`    // Lighting power density (W/m²)
	EquipmentPower   float64 `json:"equipment_power"`   // Equipment power density (W/m²)
}

// ScheduleSpec holds occupancy/lighting/equipment time schedules and
// HVAC setpoint parameters. Occupancy and HVAC availability use separate
// time windows because HVAC typically pre-conditions before occupants arrive.
type ScheduleSpec struct {
	// Occupancy / lighting / equipment active hours
	WeekdayStart string `json:"weekday_start"` // e.g. "08:00"
	WeekdayEnd   string `json:"weekday_end"`   // e.g. "18:00"
	WeekendStart string `json:"weekend_start"` // e.g. "09:00"
	WeekendEnd   string `json:"weekend_end"`   // e.g. "14:00"

	// HVAC system availability hours (usually starts earlier / ends later than occupancy)
	HVACWeekdayStart string `json:"hvac_weekday_start"` // e.g. "06:00"
	HVACWeekdayEnd   string `json:"hvac_weekday_end"`   // e.g. "22:00"
	HVACWeekendStart string `json:"hvac_weekend_start"` // e.g. "07:00"
	HVACWeekendEnd   string `json:"hvac_weekend_end"`   // e.g. "16:00"

	// Temperature setpoints
	HeatingSetpoint float64 `json:"heating_setpoint"` // Occupied heating setpoint (°C)
	CoolingSetpoint float64 `json:"cooling_setpoint"` // Occupied cooling setpoint (°C)
	HeatingSetback  float64 `json:"heating_setback"`  // Unoccupied heating setback (°C)
	CoolingSetup    float64 `json:"cooling_setup"`    // Unoccupied cooling setup (°C)
}

// SimSpec holds simulation control parameters.
type SimSpec struct {
	Year     int `json:"year"`     // Simulation year (full-year run Jan 1 – Dec 31)
	Timestep int `json:"timestep"` // Time steps per hour (4 or 6 recommended)
}

// BuildingIntent is the complete building design intent produced by the
// intent collection phase and consumed by the YAML generation phase.
type BuildingIntent struct {
	Building   BuildingInfo `json:"building"`
	Geometry   GeometrySpec `json:"geometry"`
	Envelope   EnvelopeSpec `json:"envelope"`
	Window     WindowSpec   `json:"window"`
	Loads      LoadsSpec    `json:"loads"`
	Schedule   ScheduleSpec `json:"schedule"`
	Simulation SimSpec      `json:"simulation"`
}

// IsComplete checks whether the intent contains the minimum information
// required to generate a YAML configuration.
func (b *BuildingIntent) IsComplete() (bool, []string) {
	missing := make([]string, 0)

	if b.Building.Name == "" {
		missing = append(missing, "building name")
	}
	if b.Building.City == "" {
		missing = append(missing, "city / location")
	}
	if b.Geometry.TotalArea <= 0 {
		missing = append(missing, "total floor area")
	}
	if b.Geometry.NumFloors <= 0 {
		missing = append(missing, "number of floors")
	}
	if len(b.Geometry.Zones) == 0 {
		missing = append(missing, "zone layout")
	}

	return len(missing) == 0, missing
}
