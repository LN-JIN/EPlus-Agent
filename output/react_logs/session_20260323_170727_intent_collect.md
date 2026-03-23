# ReAct 日志 — intent_collect — session_20260323_170727

## 迭代 1

**⚡ Action:** `ask_user`

**📥 Input:**
```json
{"question": "请问建筑的平面形状是矩形还是其他形状（如 L 形、U 形等）？"}
```

**📤 Observation:**
```
矩形
```

---

## 迭代 2

**⚡ Action:** `present_summary`

**📥 Input:**
```json
{"intent_json": "{\n  \"building\": {\n    \"name\": \"ShenzhenResidentialBuilding\",\n    \"type\": \"Residential\",\n    \"city\": \"Shenzhen\",\n    \"latitude\": 22.54,\n    \"longitude\": 114.00,\n    \"time_zone\": 8.0,\n    \"elevation\": 4.0,\n    \"terrain\": \"Suburbs\",\n    \"north_axis\": 0.0\n  },\n  \"geometry\": {\n    \"num_floors\": 3,\n    \"total_area\": 400.0,\n    \"floor_width\": 12.0,\n    \"floor_depth\": 12.0,\n    \"floor_height\": 3.0,\n    \"zones\": [\n      {\"name\": \"Zone_F1\", \"floor\": 1, \"multiplier\": 1},\n      {\"name\": \"Zone_F2\", \"floor\": 2, \"multiplier\": 1},\n      {\"name\": \"Zone_F3\", \"floor\": 3, \"multiplier\": 1}\n    ]\n  },\n  \"envelope\": {\n    \"wall_u\": 1.00,\n    \"roof_u\": 0.60,\n    \"floor_u\": 0.80,\n    \"insulation\": \"EPS\"\n  },\n  \"window\": {\n    \"wwr_south\": 0.35,\n    \"wwr_north\": 0.25,\n    \"wwr_east\": 0.20,\n    \"wwr_west\": 0.20,\n    \"u_factor\": 2.70,\n    \"shgc\": 0.60,\n    \"vt\": 0.72\n  },\n  \"loads\": {\n    \"occupancy_type\": \"residential\",\n    \"occupancy_density\": 0.02,\n    \"lighting_power\": 6.0,\n    \"equipment_power\": 5.0,\n    \"infiltration_ach\": 0.50\n  },\n  \"schedule\": {\n    \"weekday_start\": \"08:00\",\n    \"weekday_end\": \"22:00\",\n    \"weekend_start\": \"08:00\",\n    \"weekend_end\": \"22:00\",\n    \"hvac_weekday_start\": \"06:00\",\n    \"hvac_weekday_end\": \"23:00\",\n    \"hvac_weekend_start\": \"06:00\",\n    \"hvac_weekend_end\": \"23:00\",\n    \"heating_setpoint\": 20.0,\n    \"cooling_setpoint\": 26.0,\n    \"heating_setback\": 16.0,\n    \"cooling_setup\": 30.0\n  },\n  \"simulation\": {\n    \"year\": 2024,\n    \"timestep\": 4\n  }\n}"}
```

**📤 Observation:**
```
用户已确认，信息正确。可以开始生成 YAML 配置。
```

---

## 迭代 3

**🤔 Thought:** ```yaml
# =============================================================================
# EnergyPlus Simulation Configuration
# Building: ShenzhenResidentialBuilding
# Generated from user description: 深圳市 3 层住宅，400 m²，矩形平面
# =============================================================================

Version:
  VersionNumber: 9.6

# -----------------------------------------------------------------------------
# Building Location and Climate
# -----------------------------------------------------------------------------
Site:Location:
  Name: Shenzhen, China
  Latitude: 22.54
  Longitude: 114.00
  Time Zone: 8.0
  Elevation: 4.0

Site:GroundTemperature:BuildingSurface:
  - 19.0, 19.5, 20.5, 22.0, 24.0, 25.5, 26.0, 26.0, 25.5, 24.0, 22.0, 20.0

# -----------------------------------------------------------------------------
# Building Geometry
# -----------------------------------------------------------------------------
Building:
  Name: ShenzhenResidentialBuilding
  North Axis: 0.0
  Terrain: Suburbs
  Loads Convergence Tolerance Type: TemperatureDifference
  Loads Convergence Tolerance Value: 0.00032
  Temperature Convergence Tolerance Value: 0.0008
  Solar Distribution: FullExterior
  Maximum Number of Warmup Days: 25
  Minimum Number of Warmup Days: 6

# Floor dimensions: 400 m² / 3 floors ≈ 133.3 m²/floor
# Using 12m × 12m = 144 m² per floor (close approximation)
# Floor height: 3.0 m (Residential default)

Zone:
  - Name: Zone_F1
    Direction of Relative North: 0.0
    X Origin: 0.0
    Y Origin: 0.0
    Z Origin: 0.0
    Type: 1
    Multiplier: 1
    Ceiling Height: 3.0
    Volume: 1728.0
    Floor Area: 144.0
    Zone Inside Convection Algorithm: TARP
    Zone Outside Convection Algorithm: TARP

  - Name: Zone_F2
    Direction of Relative North: 0.0
    X Origin: 0.0
    Y Origin: 0.0
    Z Origin: 3.0
    Type: 1
    Multiplier: 1
    Ceiling Height: 3.0
    Volume: 1728.0
    Floor Area: 144.0
    Zone Inside Convection Algorithm: TARP
    Zone Outside Convection Algorithm: TARP

  - Name: Zone_F3
    Direction of Relative North: 0.0
    X Origin: 0.0
    Y Origin: 0.0
    Z Origin: 6.0
    Type: 1
    Multiplier: 1
    Ceiling Height: 3.0
    Volume: 1728.0
    Floor Area: 144.0
    Zone Inside Convection Algorithm: TARP
    Zone Outside Convection Algorithm: TARP

# -----------------------------------------------------------------------------
# Building Surfaces
# -----------------------------------------------------------------------------

# Zone_F1 Surfaces
BuildingSurface:Detailed:
  - Name: Zone_F1_Floor
    Surface Type: Floor
    Construction Name: GroundFloor
    Zone Name: Zone_F1
    Outside Boundary Condition: Ground
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 0.0
      - 12.0, 0.0, 0.0
      - 12.0, 12.0, 0.0
      - 0.0, 12.0, 0.0

  - Name: Zone_F1_Roof
    Surface Type: Ceiling
    Construction Name: InteriorFloor
    Zone Name: Zone_F1
    Outside Boundary Condition: Surface
    Outside Boundary Condition Object: Zone_F2_Floor
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 3.0
      - 12.0, 0.0, 3.0
      - 12.0, 12.0, 3.0
      - 0.0, 12.0, 3.0

  - Name: Zone_F1_Wall_South
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 0.0, 0.0
      - 12.0, 0.0, 0.0
      - 12.0, 0.0, 3.0
      - 0.0, 0.0, 3.0

  - Name: Zone_F1_Wall_North
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 0.0
      - 0.0, 12.0, 3.0
      - 12.0, 12.0, 3.0
      - 12.0, 12.0, 0.0

  - Name: Zone_F1_Wall_East
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 0.0, 0.0
      - 12.0, 12.0, 0.0
      - 12.0, 12.0, 3.0
      - 12.0, 0.0, 3.0

  - Name: Zone_F1_Wall_West
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 0.0
      - 0.0, 0.0, 0.0
      - 0.0, 0.0, 3.0
      - 0.0, 12.0, 3.0

# Zone_F2 Surfaces
BuildingSurface:Detailed:
  - Name: Zone_F2_Floor
    Surface Type: Floor
    Construction Name: InteriorFloor
    Zone Name: Zone_F2
    Outside Boundary Condition: Surface
    Outside Boundary Condition Object: Zone_F1_Roof
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 3.0
      - 12.0, 0.0, 3.0
      - 12.0, 12.0, 3.0
      - 0.0, 12.0, 3.0

  - Name: Zone_F2_Roof
    Surface Type: Ceiling
    Construction Name: InteriorFloor
    Zone Name: Zone_F2
    Outside Boundary Condition: Surface
    Outside Boundary Condition Object: Zone_F3_Floor
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 6.0
      - 12.0, 0.0, 6.0
      - 12.0, 12.0, 6.0
      - 0.0, 12.0, 6.0

  - Name: Zone_F2_Wall_South
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 0.0, 3.0
      - 12.0, 0.0, 3.0
      - 12.0, 0.0, 6.0
      - 0.0, 0.0, 6.0

  - Name: Zone_F2_Wall_North
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 3.0
      - 0.0, 12.0, 6.0
      - 12.0, 12.0, 6.0
      - 12.0, 12.0, 3.0

  - Name: Zone_F2_Wall_East
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 0.0, 3.0
      - 12.0, 12.0, 3.0
      - 12.0, 12.0, 6.0
      - 12.0, 0.0, 6.0

  - Name: Zone_F2_Wall_West
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 3.0
      - 0.0, 0.0, 3.0
      - 0.0, 0.0, 6.0
      - 0.0, 12.0, 6.0

# Zone_F3 Surfaces
BuildingSurface:Detailed:
  - Name: Zone_F3_Floor
    Surface Type: Floor
    Construction Name: InteriorFloor
    Zone Name: Zone_F3
    Outside Boundary Condition: Surface
    Outside Boundary Condition Object: Zone_F2_Roof
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 6.0
      - 12.0, 0.0, 6.0
      - 12.0, 12.0, 6.0
      - 0.0, 12.0, 6.0

  - Name: Zone_F3_Roof
    Surface Type: Roof
    Construction Name: ExteriorRoof
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 0.0, 9.0
      - 12.0, 0.0, 9.0
      - 12.0, 12.0, 9.0
      - 0.0, 12.0, 9.0

  - Name: Zone_F3_Wall_South
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 0.0, 6.0
      - 12.0, 0.0, 6.0
      - 12.0, 0.0, 9.0
      - 0.0, 0.0, 9.0

  - Name: Zone_F3_Wall_North
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 6.0
      - 0.0, 12.0, 9.0
      - 12.0, 12.0, 9.0
      - 12.0, 12.0, 6.0

  - Name: Zone_F3_Wall_East
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 0.0, 6.0
      - 12.0, 12.0, 6.0
      - 12.0, 12.0, 9.0
      - 12.0, 0.0, 9.0

  - Name: Zone_F3_Wall_West
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 6.0
      - 0.0, 0.0, 6.0
      - 0.0, 0.0, 9.0
      - 0.0, 12.0, 9.0

# -----------------------------------------------------------------------------
# Window Surfaces
# WWR: South 0.35, North 0.25, East 0.20, West 0.20
# Wall area per floor per side: 12m × 3m = 36 m²
# South window: 36 × 0.35 = 12.6 m² → 4.2m × 3.0m (centered)
# North window: 36 × 0.25 = 9.0 m² → 3.0m × 3.0m (centered)
# East window: 36 × 0.20 = 7.2 m² → 2.4m × 3.0m (centered)
# West window: 36 × 0.20 = 7.2 m² → 2.4m × 3.0m (centered)

FenestrationSurface:Detailed:
  # Zone_F1 Windows
  - Name: Zone_F1_Window_South
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 3.9, 0.0, 0.0
      - 8.1, 0.0, 0.0
      - 8.1, 0.0, 3.0
      - 3.9, 0.0, 3.0

  - Name: Zone_F1_Window_North
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 4.5, 12.0, 0.0
      - 7.5, 12.0, 0.0
      - 7.5, 12.0, 3.0
      - 4.5, 12.0, 3.0

  - Name: Zone_F1_Window_East
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 4.8, 0.0
      - 12.0, 7.2, 0.0
      - 12.0, 7.2, 3.0
      - 12.0, 4.8, 3.0

  - Name: Zone_F1_Window_West
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 4.8, 0.0
      - 0.0, 7.2, 0.0
      - 0.0, 7.2, 3.0
      - 0.0, 4.8, 3.0

  # Zone_F2 Windows
  - Name: Zone_F2_Window_South
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 3.9, 0.0, 3.0
      - 8.1, 0.0, 3.0
      - 8.1, 0.0, 6.0
      - 3.9, 0.0, 6.0

  - Name: Zone_F2_Window_North
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 4.5, 12.0, 3.0
      - 7.5, 12.0, 3.0
      - 7.5, 12.0, 6.0
      - 4.5, 12.0, 6.0

  - Name: Zone_F2_Window_East
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 4.8, 3.0
      - 12.0, 7.2, 3.0
      - 12.0, 7.2, 6.0
      - 12.0, 4.8, 6.0

  - Name: Zone_F2_Window_West
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 4.8, 3.0
      - 0.0, 7.2, 3.0
      - 0.0, 7.2, 6.0
      - 0.0, 4.8, 6.0

  # Zone_F3 Windows
  - Name: Zone_F3_Window_South
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 3.9, 0.0, 6.0
      - 8.1, 0.0, 6.0
      - 8.1, 0.0, 9.0
      - 3.9, 0.0, 9.0

  - Name: Zone_F3_Window_North
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 4.5, 12.0, 6.0
      - 7.5, 12.0, 6.0
      - 7.5, 12.0, 9.0
      - 4.5, 12.0, 9.0

  - Name: Zone_F3_Window_East
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 4.8, 6.0
      - 12.0, 7.2, 6.0
      - 12.0, 7.2, 9.0
      - 12.0, 4.8, 9.0

  - Name: Zone_F3_Window_West
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 4.8, 6.0
      - 0.0, 7.2, 6.0
      - 0.0, 7.2, 9.0
      - 0.0, 4.8, 9.0

# -----------------------------------------------------------------------------
# Constructions
# U-values: Wall=1.00, Roof=0.60, Floor=0.80, Window=2.70
# Climate Zone 4 (Hot Summer Warm Winter), Shenzhen
# Insulation: EPS (default 65% energy-saving)

Construction:
  - Name: ExteriorWall
    Outside Layer: EPS_Insulation_100mm
    Inside Layer: Concrete_200mm

  - Name: ExteriorRoof
    Outside Layer: EPS_Insulation_120mm
    Inside Layer: Concrete_150mm

  - Name: InteriorFloor
    Outside Layer: Concrete_150mm
    Inside Layer: Concrete_150mm

  - Name: GroundFloor
    Outside Layer: Concrete_200mm
    Inside Layer: EPS_Insulation_80mm

  - Name: ExteriorWindow
    Outside Layer: Clear_3mm
    Inside Layer: Air_12mm
    Inside Layer: Clear_3mm

# -----------------------------------------------------------------------------
# Materials
# -----------------------------------------------------------------------------

Material:
  - Name: EPS_Insulation_100mm
    Roughness: MediumSmooth
    Thickness: 0.10
    Conductivity: 0.038
    Density: 30.0
    Specific Heat: 1400.0

  - Name: EPS_Insulation_120mm
    Roughness: MediumSmooth
    Thickness: 0.12
    Conductivity: 0.038
    Density: 30.0
    Specific Heat: 1400.0

  - Name: EPS_Insulation_80mm
    Roughness: MediumSmooth
    Thickness: 0.08
    Conductivity: 0.038
    Density: 30.0
    Specific Heat: 1400.0

  - Name: Concrete_200mm
    Roughness: MediumRough
    Thickness: 0.20
    Conductivity: 1.75
    Density: 2400.0
    Specific Heat: 920.0

  - Name: Concrete_150mm
    Roughness: MediumRough
    Thickness: 0.15
    Conductivity: 1.75
    Density: 2400.0
    Specific Heat: 920.0

Material:NoMass:
  - Name: Clear_3mm
    Roughness: VerySmooth
    Solar Absorptance: 0.08
    Visible Absorptance: 0.08
    Thermal Resistance: 0.0
    Thermal Absorptance: 0.90

Material:NoMass:
  - Name: Air_12mm
    Roughness: Smooth
    Solar Absorptance: 0.0
    Visible Absorptance: 0.0
    Thermal Resistance: 0.17
    Thermal Absorptance: 0.0

# -----------------------------------------------------------------------------
# Internal Loads
# Occupancy: 0.02 ppl/m², Lighting: 6 W/m², Equipment: 5 W/m²
# -----------------------------------------------------------------------------

People:
  - Name: Zone_F1_People
    Zone Name: Zone_F1
    People Schedule Name: Residential Occupancy
    Number of People Schedule Name: 
    Number of People Calculation Method: People/Area
    People per Area Floor Area: 0.02
    Activity Level Schedule Name: Residential Activity
    Sensible Heat Fraction: 0.7
    Radiant Heat Fraction: 0.2
    Latent Heat Fraction: 0.1
    Fraction Radiant: 0.2
    Fraction Convect: 0.5
    Fraction Replaceable: 0.0

  - Name: Zone_F2_People
    Zone Name: Zone_F2
    People Schedule Name: Residential Occupancy
    Number of People Schedule Name: 
    Number of People Calculation Method: People/Area
    People per Area Floor Area: 0.02
    Activity Level Schedule Name: Residential Activity
    Sensible Heat Fraction: 0.7
    Radiant Heat Fraction: 0.2
    Latent Heat Fraction: 0.1
    Fraction Radiant: 0.2
    Fraction Convect: 0.5
    Fraction Replaceable: 0.0

  - Name: Zone_F3_People
    Zone Name: Zone_F3
    People Schedule Name: Residential Occupancy
    Number of People Schedule Name: 
    Number of People Calculation Method: People/Area
    People per Area Floor Area: 0.02
    Activity Level Schedule Name: Residential Activity
    Sensible Heat Fraction: 0.7
    Radiant Heat Fraction: 0.2
    Latent Heat Fraction: 0.1
    Fraction Radiant: 0.2
    Fraction Convect: 0.5
    Fraction Replaceable: 0.0

Lights:
  - Name: Zone_F1_Lights
    Zone Name: Zone_F1
    Schedule Name: Residential Lighting
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 6.0
    Return Air Fraction: 0.0
    Return Air Fraction Calculated from Plenum Temperature: No
    Fraction Radiant: 0.6
    Fraction Visible: 0.2
    Fraction Replaceable: 1.0

  - Name: Zone_F2_Lights
    Zone Name: Zone_F2
    Schedule Name: Residential Lighting
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 6.0
    Return Air Fraction: 0.0
    Return Air Fraction Calculated from Plenum Temperature: No
    Fraction Radiant: 0.6
    Fraction Visible: 0.2
    Fraction Replaceable: 1.0

  - Name: Zone_F3_Lights
    Zone Name: Zone_F3
    Schedule Name: Residential Lighting
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 6.0
    Return Air Fraction: 0.0
    Return Air Fraction Calculated from Plenum Temperature: No
    Fraction Radiant: 0.6
    Fraction Visible: 0.2
    Fraction Replaceable: 1.0

ElectricEquipment:
  - Name: Zone_F1_Equipment
    Zone Name: Zone_F1
    Schedule Name: Residential Equipment
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 5.0
    Fraction Radiant: 0.5
    Fraction Latent: 0.0
    Fraction Lost: 0.0
    Fraction Replaceable: 1.0

  - Name: Zone_F2_Equipment
    Zone Name: Zone_F2
    Schedule Name: Residential Equipment
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 5.0
    Fraction Radiant: 0.5
    Fraction Latent: 0.0
    Fraction Lost: 0.0
    Fraction Replaceable: 1.0

  - Name: Zone_F3_Equipment
    Zone Name: Zone_F3
    Schedule Name: Residential Equipment
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 5.0
    Fraction Radiant: 0.5
    Fraction Latent: 0.0
    Fraction Lost: 0.0
    Fraction Replaceable: 1.0

# -----------------------------------------------------------------------------
# Infiltration
# 0.50 ACH (Residential default for Zone 4)
# -----------------------------------------------------------------------------

ZoneInfiltration:DesignFlowRate:
  - Name: Zone_F1_Infiltration
    Zone Name: Zone_F1
    Schedule Name: Residential Infiltration
    Design Flow Rate Calculation Method: AirChanges/Hour
    Air Changes per Hour: 0.50
    Constant Term Coefficient: 1.0
    Temperature Term Coefficient: 0.0
    Velocity Term Coefficient: 0.0
    Velocity Squared Term Coefficient: 0.0

  - Name: Zone_F2_Infiltration
    Zone Name: Zone_F2
    Schedule Name: Residential Infiltration
    Design Flow Rate Calculation Method: AirChanges/Hour
    Air Changes per Hour: 0.50
    Constant Term Coefficient: 1.0
    Temperature Term Coefficient: 0.0
    Velocity Term Coefficient: 0.0
    Velocity Squared Term Coefficient: 0.0

  - Name: Zone_F3_Infiltration
    Zone Name: Zone_F3
    Schedule Name: Residential Infiltration
    Design Flow Rate Calculation Method: AirChanges/Hour
    Air Changes per Hour: 0.50
    Constant Term Coefficient: 1.0
    Temperature Term Coefficient: 0.0
    Velocity Term Coefficient: 0.0
    Velocity Squared Term Coefficient: 0.0

# -----------------------------------------------------------------------------
# Schedules
# -----------------------------------------------------------------------------

Schedule:Compact:
  - Name: Residential Occupancy
    Schedule Type Limits Name: Fraction
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 08:00, 0.0
      - Until: 22:00, 1.0
      - Until: 24:00, 0.0

  - Name: Residential Lighting
    Schedule Type Limits Name: Fraction
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 08:00, 0.0
      - Until: 22:00, 1.0
      - Until: 24:00, 0.0

  - Name: Residential Equipment
    Schedule Type Limits Name: Fraction
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 08:00, 0.0
      - Until: 22:00, 1.0
      - Until: 24:00, 0.0

  - Name: Residential Infiltration
    Schedule Type Limits Name: Fraction
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 24:00, 1.0

  - Name: Residential Activity
    Schedule Type Limits Name: ActivityLevel
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 24:00, 120.0

  - Name: HVAC Operation
    Schedule Type Limits Name: On/Off
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 06:00, 0.0
      - Until: 23:00, 1.0
      - Until: 24:00, 0.0

ScheduleTypeLimits:
  - Name: Fraction
    Lower Limit Value: 0.0
    Upper Limit Value: 1.0
    Numeric Type: Continuous

  - Name: On/Off
    Lower Limit Value: 0.0
    Upper Limit Value: 1.0
    Numeric Type: Discrete

  - Name: ActivityLevel
    Lower Limit Value: 0.0
    Upper Limit Value: 500.0
    Numeric Type: Continuous

# -----------------------------------------------------------------------------
# Thermostat and HVAC
# Heating Setpoint: 20°C, Cooling Setpoint: 26°C
# Heating Setback: 16°C, Cooling Setup: 30°C
# -----------------------------------------------------------------------------

ZoneControl:Thermostat:
  - Name: Zone_F1_Thermostat
    Zone Name: Zone_F1
    Control Type Schedule Name: HVAC Operation
    Single Heating Setpoint Temperature Schedule Name: Heating Setpoint
    Single Cooling Setpoint Temperature Schedule Name: Cooling Setpoint

  - Name: Zone_F2_Thermostat
    Zone Name: Zone_F2
    Control Type Schedule Name: HVAC Operation
    Single Heating Setpoint Temperature Schedule Name: Heating Setpoint
    Single Cooling Setpoint Temperature Schedule Name: Cooling Setpoint

  - Name: Zone_F3_Thermostat
    Zone Name: Zone_F3
    Control Type Schedule Name: HVAC Operation
    Single Heating Setpoint Temperature Schedule Name: Heating Setpoint
    Single Cooling Setpoint Temperature Schedule Name: Cooling Setpoint

Schedule:Compact:
  - Name: Heating Setpoint
    Schedule Type Limits Name: Temperature
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 06:00, 16.0
      - Until: 23:00, 20.0
      - Until: 24:00, 16.0

  - Name: Cooling Setpoint
    Schedule Type Limits Name: Temperature
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 06:00, 30.0
      - Until: 23:00, 26.0
      - Until: 24:00, 30.0

ScheduleTypeLimits:
  - Name: Temperature
    Lower Limit Value: -100.0
    Upper Limit Value: 200.0
    Numeric Type: Continuous

# Ideal Loads Air System (simplified HVAC)
ZoneHVAC:IdealLoadsAirSystem:
  - Name: Zone_F1_IdealAir
    Zone Name: Zone_F1
    Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Temperature: 50.0
    Minimum Heating Supply Air Temperature: 10.0
    Maximum Cooling Supply Air Temperature: 15.0
    Minimum Cooling Supply Air Temperature: 10.0
    Heating Availability Schedule Name: HVAC Operation
    Cooling Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Humidity Ratio: 0.015
    Minimum Heating Supply Air Humidity Ratio: 0.0
    Maximum Cooling Supply Air Humidity Ratio: 0.008
    Minimum Cooling Supply Air Humidity Ratio: 0.0
    Heating Limit: NoLimit
    Maximum Heating Air Flow Rate: 
    Maximum Sensible Heating Capacity: 
    Cooling Limit: NoLimit
    Maximum Cooling Air Flow Rate: 
    Maximum Total Cooling Capacity: 
    Dehumidification Control Type: None
    Humidification Control Type: None
    Cooling Supply Air Path Name: 
    Heating Supply Air Path Name: 
    Outdoor Air Path Name: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Outdoor Air Flow Rate per Person: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Heat Recovery Type: None
    Sensible Heat Recovery Effectiveness: 0.7
    Latent Heat Recovery Effectiveness: 0.65
    Time of Day Heat Recovery Availability Schedule Name: 
    Economizer Control Type: NoEconomizer
    Economizer Control Action Type: 
    Economizer Maximum Limit Dry-Bulb Temperature: 
    Economizer Maximum Limit Enthalpy: 
    Economizer Maximum Limit Dewpoint Temperature: 
    Economizer Minimum Limit Dry-Bulb Temperature: 

  - Name: Zone_F2_IdealAir
    Zone Name: Zone_F2
    Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Temperature: 50.0
    Minimum Heating Supply Air Temperature: 10.0
    Maximum Cooling Supply Air Temperature: 15.0
    Minimum Cooling Supply Air Temperature: 10.0
    Heating Availability Schedule Name: HVAC Operation
    Cooling Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Humidity Ratio: 0.015
    Minimum Heating Supply Air Humidity Ratio: 0.0
    Maximum Cooling Supply Air Humidity Ratio: 0.008
    Minimum Cooling Supply Air Humidity Ratio: 0.0
    Heating Limit: NoLimit
    Maximum Heating Air Flow Rate: 
    Maximum Sensible Heating Capacity: 
    Cooling Limit: NoLimit
    Maximum Cooling Air Flow Rate: 
    Maximum Total Cooling Capacity: 
    Dehumidification Control Type: None
    Humidification Control Type: None
    Cooling Supply Air Path Name: 
    Heating Supply Air Path Name: 
    Outdoor Air Path Name: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Outdoor Air Flow Rate per Person: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Heat Recovery Type: None
    Sensible Heat Recovery Effectiveness: 0.7
    Latent Heat Recovery Effectiveness: 0.65
    Time of Day Heat Recovery Availability Schedule Name: 
    Economizer Control Type: NoEconomizer
    Economizer Control Action Type: 
    Economizer Maximum Limit Dry-Bulb Temperature: 
    Economizer Maximum Limit Enthalpy: 
    Economizer Maximum Limit Dewpoint Temperature: 
    Economizer Minimum Limit Dry-Bulb Temperature: 

  - Name: Zone_F3_IdealAir
    Zone Name: Zone_F3
    Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Temperature: 50.0
    Minimum Heating Supply Air Temperature: 10.0
    Maximum Cooling Supply Air Temperature: 15.0
    Minimum Cooling Supply Air Temperature: 10.0
    Heating Availability Schedule Name: HVAC Operation
    Cooling Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Humidity Ratio: 0.015
    Minimum Heating Supply Air Humidity Ratio: 0.0
    Maximum Cooling Supply Air Humidity Ratio: 0.008
    Minimum Cooling Supply Air Humidity Ratio: 0.0
    Heating Limit: NoLimit
    Maximum Heating Air Flow Rate: 
    Maximum Sensible Heating Capacity: 
    Cooling Limit: NoLimit
    Maximum Cooling Air Flow Rate: 
    Maximum Total Cooling Capacity: 
    Dehumidification Control Type: None
    Humidification Control Type: None
    Cooling Supply Air Path Name: 
    Heating Supply Air Path Name: 
    Outdoor Air Path Name: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Outdoor Air Flow Rate per Person: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Heat Recovery Type: None
    Sensible Heat Recovery Effectiveness: 0.7
    Latent Heat Recovery Effectiveness: 0.65
    Time of Day Heat Recovery Availability Schedule Name: 
    Economizer Control Type: NoEconomizer
    Economizer Control Action Type: 
    Economizer Maximum Limit Dry-Bulb Temperature: 
    Economizer Maximum Limit Enthalpy: 
    Economizer Maximum Limit Dewpoint Temperature: 
    Economizer Minimum Limit Dry-Bulb Temperature: 

# -----------------------------------------------------------------------------
# Output Control
# -----------------------------------------------------------------------------

OutputControl:ReportingTolerances:
  Column Tolerance: 0.00001

Output:Table:SummaryReports:
  Report Name 0: AllSummary

Output:Meter:
  - Name: Electricity:Facility
    Reporting Frequency: Hourly

Output:Meter:
  - Name: Gas:Facility
    Reporting Frequency: Hourly

Output:Meter:
  - Name: EnergyTransfer:Facility
    Reporting Frequency: Hourly

Output:Meter:
  - Name: Zone Air Temperature
    Reporting Frequency: Hourly

Output:Meter:
  - Name: Zone Air Relative Humidity
    Reporting Frequency: Hourly

# -----------------------------------------------------------------------------
# Simulation Control
# -----------------------------------------------------------------------------

SimulationControl:
  Do Zone Sizing Calculation: Yes
  Do System Sizing Calculation: Yes
  Do Plant Sizing Calculation: No
  Run Simulation for Sizing Periods: No
  Run Simulation for Weather File Run Periods: Yes

RunPeriod:
  - Name: Full Year
    Begin Month: 1
    Begin Day of Month: 1
    End Month: 12
    End Day of Month: 31
    Year: 2024
    Day of Week for Start Day: Monday
    Use Weather File Holidays and Special Days: Yes
    Use Weather File Daylight Saving Period: Yes
    Apply Weekend Holiday Rule: Yes
    Use Weather File Rain Indicators: Yes
    Use Weather File Snow Indicators: Yes

Timestep:
  Number of Timesteps per Hour: 4

# =============================================================================
# End of EnergyPlus Configuration
# =============================================================================
```

**✅ Final Answer:** ```yaml
# =============================================================================
# EnergyPlus Simulation Configuration
# Building: ShenzhenResidentialBuilding
# Generated from user description: 深圳市 3 层住宅，400 m²，矩形平面
# =============================================================================

Version:
  VersionNumber: 9.6

# -----------------------------------------------------------------------------
# Building Location and Climate
# -----------------------------------------------------------------------------
Site:Location:
  Name: Shenzhen, China
  Latitude: 22.54
  Longitude: 114.00
  Time Zone: 8.0
  Elevation: 4.0

Site:GroundTemperature:BuildingSurface:
  - 19.0, 19.5, 20.5, 22.0, 24.0, 25.5, 26.0, 26.0, 25.5, 24.0, 22.0, 20.0

# -----------------------------------------------------------------------------
# Building Geometry
# -----------------------------------------------------------------------------
Building:
  Name: ShenzhenResidentialBuilding
  North Axis: 0.0
  Terrain: Suburbs
  Loads Convergence Tolerance Type: TemperatureDifference
  Loads Convergence Tolerance Value: 0.00032
  Temperature Convergence Tolerance Value: 0.0008
  Solar Distribution: FullExterior
  Maximum Number of Warmup Days: 25
  Minimum Number of Warmup Days: 6

# Floor dimensions: 400 m² / 3 floors ≈ 133.3 m²/floor
# Using 12m × 12m = 144 m² per floor (close approximation)
# Floor height: 3.0 m (Residential default)

Zone:
  - Name: Zone_F1
    Direction of Relative North: 0.0
    X Origin: 0.0
    Y Origin: 0.0
    Z Origin: 0.0
    Type: 1
    Multiplier: 1
    Ceiling Height: 3.0
    Volume: 1728.0
    Floor Area: 144.0
    Zone Inside Convection Algorithm: TARP
    Zone Outside Convection Algorithm: TARP

  - Name: Zone_F2
    Direction of Relative North: 0.0
    X Origin: 0.0
    Y Origin: 0.0
    Z Origin: 3.0
    Type: 1
    Multiplier: 1
    Ceiling Height: 3.0
    Volume: 1728.0
    Floor Area: 144.0
    Zone Inside Convection Algorithm: TARP
    Zone Outside Convection Algorithm: TARP

  - Name: Zone_F3
    Direction of Relative North: 0.0
    X Origin: 0.0
    Y Origin: 0.0
    Z Origin: 6.0
    Type: 1
    Multiplier: 1
    Ceiling Height: 3.0
    Volume: 1728.0
    Floor Area: 144.0
    Zone Inside Convection Algorithm: TARP
    Zone Outside Convection Algorithm: TARP

# -----------------------------------------------------------------------------
# Building Surfaces
# -----------------------------------------------------------------------------

# Zone_F1 Surfaces
BuildingSurface:Detailed:
  - Name: Zone_F1_Floor
    Surface Type: Floor
    Construction Name: GroundFloor
    Zone Name: Zone_F1
    Outside Boundary Condition: Ground
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 0.0
      - 12.0, 0.0, 0.0
      - 12.0, 12.0, 0.0
      - 0.0, 12.0, 0.0

  - Name: Zone_F1_Roof
    Surface Type: Ceiling
    Construction Name: InteriorFloor
    Zone Name: Zone_F1
    Outside Boundary Condition: Surface
    Outside Boundary Condition Object: Zone_F2_Floor
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 3.0
      - 12.0, 0.0, 3.0
      - 12.0, 12.0, 3.0
      - 0.0, 12.0, 3.0

  - Name: Zone_F1_Wall_South
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 0.0, 0.0
      - 12.0, 0.0, 0.0
      - 12.0, 0.0, 3.0
      - 0.0, 0.0, 3.0

  - Name: Zone_F1_Wall_North
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 0.0
      - 0.0, 12.0, 3.0
      - 12.0, 12.0, 3.0
      - 12.0, 12.0, 0.0

  - Name: Zone_F1_Wall_East
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 0.0, 0.0
      - 12.0, 12.0, 0.0
      - 12.0, 12.0, 3.0
      - 12.0, 0.0, 3.0

  - Name: Zone_F1_Wall_West
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 0.0
      - 0.0, 0.0, 0.0
      - 0.0, 0.0, 3.0
      - 0.0, 12.0, 3.0

# Zone_F2 Surfaces
BuildingSurface:Detailed:
  - Name: Zone_F2_Floor
    Surface Type: Floor
    Construction Name: InteriorFloor
    Zone Name: Zone_F2
    Outside Boundary Condition: Surface
    Outside Boundary Condition Object: Zone_F1_Roof
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 3.0
      - 12.0, 0.0, 3.0
      - 12.0, 12.0, 3.0
      - 0.0, 12.0, 3.0

  - Name: Zone_F2_Roof
    Surface Type: Ceiling
    Construction Name: InteriorFloor
    Zone Name: Zone_F2
    Outside Boundary Condition: Surface
    Outside Boundary Condition Object: Zone_F3_Floor
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 6.0
      - 12.0, 0.0, 6.0
      - 12.0, 12.0, 6.0
      - 0.0, 12.0, 6.0

  - Name: Zone_F2_Wall_South
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 0.0, 3.0
      - 12.0, 0.0, 3.0
      - 12.0, 0.0, 6.0
      - 0.0, 0.0, 6.0

  - Name: Zone_F2_Wall_North
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 3.0
      - 0.0, 12.0, 6.0
      - 12.0, 12.0, 6.0
      - 12.0, 12.0, 3.0

  - Name: Zone_F2_Wall_East
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 0.0, 3.0
      - 12.0, 12.0, 3.0
      - 12.0, 12.0, 6.0
      - 12.0, 0.0, 6.0

  - Name: Zone_F2_Wall_West
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 3.0
      - 0.0, 0.0, 3.0
      - 0.0, 0.0, 6.0
      - 0.0, 12.0, 6.0

# Zone_F3 Surfaces
BuildingSurface:Detailed:
  - Name: Zone_F3_Floor
    Surface Type: Floor
    Construction Name: InteriorFloor
    Zone Name: Zone_F3
    Outside Boundary Condition: Surface
    Outside Boundary Condition Object: Zone_F2_Roof
    Sun Exposure: No
    Wind Exposure: No
    Vertices:
      - 0.0, 0.0, 6.0
      - 12.0, 0.0, 6.0
      - 12.0, 12.0, 6.0
      - 0.0, 12.0, 6.0

  - Name: Zone_F3_Roof
    Surface Type: Roof
    Construction Name: ExteriorRoof
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 0.0, 9.0
      - 12.0, 0.0, 9.0
      - 12.0, 12.0, 9.0
      - 0.0, 12.0, 9.0

  - Name: Zone_F3_Wall_South
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 0.0, 6.0
      - 12.0, 0.0, 6.0
      - 12.0, 0.0, 9.0
      - 0.0, 0.0, 9.0

  - Name: Zone_F3_Wall_North
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 6.0
      - 0.0, 12.0, 9.0
      - 12.0, 12.0, 9.0
      - 12.0, 12.0, 6.0

  - Name: Zone_F3_Wall_East
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 0.0, 6.0
      - 12.0, 12.0, 6.0
      - 12.0, 12.0, 9.0
      - 12.0, 0.0, 9.0

  - Name: Zone_F3_Wall_West
    Surface Type: Wall
    Construction Name: ExteriorWall
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 12.0, 6.0
      - 0.0, 0.0, 6.0
      - 0.0, 0.0, 9.0
      - 0.0, 12.0, 9.0

# -----------------------------------------------------------------------------
# Window Surfaces
# WWR: South 0.35, North 0.25, East 0.20, West 0.20
# Wall area per floor per side: 12m × 3m = 36 m²
# South window: 36 × 0.35 = 12.6 m² → 4.2m × 3.0m (centered)
# North window: 36 × 0.25 = 9.0 m² → 3.0m × 3.0m (centered)
# East window: 36 × 0.20 = 7.2 m² → 2.4m × 3.0m (centered)
# West window: 36 × 0.20 = 7.2 m² → 2.4m × 3.0m (centered)

FenestrationSurface:Detailed:
  # Zone_F1 Windows
  - Name: Zone_F1_Window_South
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 3.9, 0.0, 0.0
      - 8.1, 0.0, 0.0
      - 8.1, 0.0, 3.0
      - 3.9, 0.0, 3.0

  - Name: Zone_F1_Window_North
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 4.5, 12.0, 0.0
      - 7.5, 12.0, 0.0
      - 7.5, 12.0, 3.0
      - 4.5, 12.0, 3.0

  - Name: Zone_F1_Window_East
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 4.8, 0.0
      - 12.0, 7.2, 0.0
      - 12.0, 7.2, 3.0
      - 12.0, 4.8, 3.0

  - Name: Zone_F1_Window_West
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F1
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 4.8, 0.0
      - 0.0, 7.2, 0.0
      - 0.0, 7.2, 3.0
      - 0.0, 4.8, 3.0

  # Zone_F2 Windows
  - Name: Zone_F2_Window_South
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 3.9, 0.0, 3.0
      - 8.1, 0.0, 3.0
      - 8.1, 0.0, 6.0
      - 3.9, 0.0, 6.0

  - Name: Zone_F2_Window_North
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 4.5, 12.0, 3.0
      - 7.5, 12.0, 3.0
      - 7.5, 12.0, 6.0
      - 4.5, 12.0, 6.0

  - Name: Zone_F2_Window_East
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 4.8, 3.0
      - 12.0, 7.2, 3.0
      - 12.0, 7.2, 6.0
      - 12.0, 4.8, 6.0

  - Name: Zone_F2_Window_West
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F2
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 4.8, 3.0
      - 0.0, 7.2, 3.0
      - 0.0, 7.2, 6.0
      - 0.0, 4.8, 6.0

  # Zone_F3 Windows
  - Name: Zone_F3_Window_South
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 3.9, 0.0, 6.0
      - 8.1, 0.0, 6.0
      - 8.1, 0.0, 9.0
      - 3.9, 0.0, 9.0

  - Name: Zone_F3_Window_North
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 4.5, 12.0, 6.0
      - 7.5, 12.0, 6.0
      - 7.5, 12.0, 9.0
      - 4.5, 12.0, 9.0

  - Name: Zone_F3_Window_East
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 12.0, 4.8, 6.0
      - 12.0, 7.2, 6.0
      - 12.0, 7.2, 9.0
      - 12.0, 4.8, 9.0

  - Name: Zone_F3_Window_West
    Surface Type: Window
    Construction Name: ExteriorWindow
    Zone Name: Zone_F3
    Outside Boundary Condition: Outdoors
    Sun Exposure: Yes
    Wind Exposure: Yes
    Vertices:
      - 0.0, 4.8, 6.0
      - 0.0, 7.2, 6.0
      - 0.0, 7.2, 9.0
      - 0.0, 4.8, 9.0

# -----------------------------------------------------------------------------
# Constructions
# U-values: Wall=1.00, Roof=0.60, Floor=0.80, Window=2.70
# Climate Zone 4 (Hot Summer Warm Winter), Shenzhen
# Insulation: EPS (default 65% energy-saving)

Construction:
  - Name: ExteriorWall
    Outside Layer: EPS_Insulation_100mm
    Inside Layer: Concrete_200mm

  - Name: ExteriorRoof
    Outside Layer: EPS_Insulation_120mm
    Inside Layer: Concrete_150mm

  - Name: InteriorFloor
    Outside Layer: Concrete_150mm
    Inside Layer: Concrete_150mm

  - Name: GroundFloor
    Outside Layer: Concrete_200mm
    Inside Layer: EPS_Insulation_80mm

  - Name: ExteriorWindow
    Outside Layer: Clear_3mm
    Inside Layer: Air_12mm
    Inside Layer: Clear_3mm

# -----------------------------------------------------------------------------
# Materials
# -----------------------------------------------------------------------------

Material:
  - Name: EPS_Insulation_100mm
    Roughness: MediumSmooth
    Thickness: 0.10
    Conductivity: 0.038
    Density: 30.0
    Specific Heat: 1400.0

  - Name: EPS_Insulation_120mm
    Roughness: MediumSmooth
    Thickness: 0.12
    Conductivity: 0.038
    Density: 30.0
    Specific Heat: 1400.0

  - Name: EPS_Insulation_80mm
    Roughness: MediumSmooth
    Thickness: 0.08
    Conductivity: 0.038
    Density: 30.0
    Specific Heat: 1400.0

  - Name: Concrete_200mm
    Roughness: MediumRough
    Thickness: 0.20
    Conductivity: 1.75
    Density: 2400.0
    Specific Heat: 920.0

  - Name: Concrete_150mm
    Roughness: MediumRough
    Thickness: 0.15
    Conductivity: 1.75
    Density: 2400.0
    Specific Heat: 920.0

Material:NoMass:
  - Name: Clear_3mm
    Roughness: VerySmooth
    Solar Absorptance: 0.08
    Visible Absorptance: 0.08
    Thermal Resistance: 0.0
    Thermal Absorptance: 0.90

Material:NoMass:
  - Name: Air_12mm
    Roughness: Smooth
    Solar Absorptance: 0.0
    Visible Absorptance: 0.0
    Thermal Resistance: 0.17
    Thermal Absorptance: 0.0

# -----------------------------------------------------------------------------
# Internal Loads
# Occupancy: 0.02 ppl/m², Lighting: 6 W/m², Equipment: 5 W/m²
# -----------------------------------------------------------------------------

People:
  - Name: Zone_F1_People
    Zone Name: Zone_F1
    People Schedule Name: Residential Occupancy
    Number of People Schedule Name: 
    Number of People Calculation Method: People/Area
    People per Area Floor Area: 0.02
    Activity Level Schedule Name: Residential Activity
    Sensible Heat Fraction: 0.7
    Radiant Heat Fraction: 0.2
    Latent Heat Fraction: 0.1
    Fraction Radiant: 0.2
    Fraction Convect: 0.5
    Fraction Replaceable: 0.0

  - Name: Zone_F2_People
    Zone Name: Zone_F2
    People Schedule Name: Residential Occupancy
    Number of People Schedule Name: 
    Number of People Calculation Method: People/Area
    People per Area Floor Area: 0.02
    Activity Level Schedule Name: Residential Activity
    Sensible Heat Fraction: 0.7
    Radiant Heat Fraction: 0.2
    Latent Heat Fraction: 0.1
    Fraction Radiant: 0.2
    Fraction Convect: 0.5
    Fraction Replaceable: 0.0

  - Name: Zone_F3_People
    Zone Name: Zone_F3
    People Schedule Name: Residential Occupancy
    Number of People Schedule Name: 
    Number of People Calculation Method: People/Area
    People per Area Floor Area: 0.02
    Activity Level Schedule Name: Residential Activity
    Sensible Heat Fraction: 0.7
    Radiant Heat Fraction: 0.2
    Latent Heat Fraction: 0.1
    Fraction Radiant: 0.2
    Fraction Convect: 0.5
    Fraction Replaceable: 0.0

Lights:
  - Name: Zone_F1_Lights
    Zone Name: Zone_F1
    Schedule Name: Residential Lighting
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 6.0
    Return Air Fraction: 0.0
    Return Air Fraction Calculated from Plenum Temperature: No
    Fraction Radiant: 0.6
    Fraction Visible: 0.2
    Fraction Replaceable: 1.0

  - Name: Zone_F2_Lights
    Zone Name: Zone_F2
    Schedule Name: Residential Lighting
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 6.0
    Return Air Fraction: 0.0
    Return Air Fraction Calculated from Plenum Temperature: No
    Fraction Radiant: 0.6
    Fraction Visible: 0.2
    Fraction Replaceable: 1.0

  - Name: Zone_F3_Lights
    Zone Name: Zone_F3
    Schedule Name: Residential Lighting
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 6.0
    Return Air Fraction: 0.0
    Return Air Fraction Calculated from Plenum Temperature: No
    Fraction Radiant: 0.6
    Fraction Visible: 0.2
    Fraction Replaceable: 1.0

ElectricEquipment:
  - Name: Zone_F1_Equipment
    Zone Name: Zone_F1
    Schedule Name: Residential Equipment
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 5.0
    Fraction Radiant: 0.5
    Fraction Latent: 0.0
    Fraction Lost: 0.0
    Fraction Replaceable: 1.0

  - Name: Zone_F2_Equipment
    Zone Name: Zone_F2
    Schedule Name: Residential Equipment
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 5.0
    Fraction Radiant: 0.5
    Fraction Latent: 0.0
    Fraction Lost: 0.0
    Fraction Replaceable: 1.0

  - Name: Zone_F3_Equipment
    Zone Name: Zone_F3
    Schedule Name: Residential Equipment
    Design Level Calculation Method: Watts/Area
    Watts per Area Floor Area: 5.0
    Fraction Radiant: 0.5
    Fraction Latent: 0.0
    Fraction Lost: 0.0
    Fraction Replaceable: 1.0

# -----------------------------------------------------------------------------
# Infiltration
# 0.50 ACH (Residential default for Zone 4)
# -----------------------------------------------------------------------------

ZoneInfiltration:DesignFlowRate:
  - Name: Zone_F1_Infiltration
    Zone Name: Zone_F1
    Schedule Name: Residential Infiltration
    Design Flow Rate Calculation Method: AirChanges/Hour
    Air Changes per Hour: 0.50
    Constant Term Coefficient: 1.0
    Temperature Term Coefficient: 0.0
    Velocity Term Coefficient: 0.0
    Velocity Squared Term Coefficient: 0.0

  - Name: Zone_F2_Infiltration
    Zone Name: Zone_F2
    Schedule Name: Residential Infiltration
    Design Flow Rate Calculation Method: AirChanges/Hour
    Air Changes per Hour: 0.50
    Constant Term Coefficient: 1.0
    Temperature Term Coefficient: 0.0
    Velocity Term Coefficient: 0.0
    Velocity Squared Term Coefficient: 0.0

  - Name: Zone_F3_Infiltration
    Zone Name: Zone_F3
    Schedule Name: Residential Infiltration
    Design Flow Rate Calculation Method: AirChanges/Hour
    Air Changes per Hour: 0.50
    Constant Term Coefficient: 1.0
    Temperature Term Coefficient: 0.0
    Velocity Term Coefficient: 0.0
    Velocity Squared Term Coefficient: 0.0

# -----------------------------------------------------------------------------
# Schedules
# -----------------------------------------------------------------------------

Schedule:Compact:
  - Name: Residential Occupancy
    Schedule Type Limits Name: Fraction
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 08:00, 0.0
      - Until: 22:00, 1.0
      - Until: 24:00, 0.0

  - Name: Residential Lighting
    Schedule Type Limits Name: Fraction
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 08:00, 0.0
      - Until: 22:00, 1.0
      - Until: 24:00, 0.0

  - Name: Residential Equipment
    Schedule Type Limits Name: Fraction
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 08:00, 0.0
      - Until: 22:00, 1.0
      - Until: 24:00, 0.0

  - Name: Residential Infiltration
    Schedule Type Limits Name: Fraction
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 24:00, 1.0

  - Name: Residential Activity
    Schedule Type Limits Name: ActivityLevel
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 24:00, 120.0

  - Name: HVAC Operation
    Schedule Type Limits Name: On/Off
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 06:00, 0.0
      - Until: 23:00, 1.0
      - Until: 24:00, 0.0

ScheduleTypeLimits:
  - Name: Fraction
    Lower Limit Value: 0.0
    Upper Limit Value: 1.0
    Numeric Type: Continuous

  - Name: On/Off
    Lower Limit Value: 0.0
    Upper Limit Value: 1.0
    Numeric Type: Discrete

  - Name: ActivityLevel
    Lower Limit Value: 0.0
    Upper Limit Value: 500.0
    Numeric Type: Continuous

# -----------------------------------------------------------------------------
# Thermostat and HVAC
# Heating Setpoint: 20°C, Cooling Setpoint: 26°C
# Heating Setback: 16°C, Cooling Setup: 30°C
# -----------------------------------------------------------------------------

ZoneControl:Thermostat:
  - Name: Zone_F1_Thermostat
    Zone Name: Zone_F1
    Control Type Schedule Name: HVAC Operation
    Single Heating Setpoint Temperature Schedule Name: Heating Setpoint
    Single Cooling Setpoint Temperature Schedule Name: Cooling Setpoint

  - Name: Zone_F2_Thermostat
    Zone Name: Zone_F2
    Control Type Schedule Name: HVAC Operation
    Single Heating Setpoint Temperature Schedule Name: Heating Setpoint
    Single Cooling Setpoint Temperature Schedule Name: Cooling Setpoint

  - Name: Zone_F3_Thermostat
    Zone Name: Zone_F3
    Control Type Schedule Name: HVAC Operation
    Single Heating Setpoint Temperature Schedule Name: Heating Setpoint
    Single Cooling Setpoint Temperature Schedule Name: Cooling Setpoint

Schedule:Compact:
  - Name: Heating Setpoint
    Schedule Type Limits Name: Temperature
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 06:00, 16.0
      - Until: 23:00, 20.0
      - Until: 24:00, 16.0

  - Name: Cooling Setpoint
    Schedule Type Limits Name: Temperature
    Field: 1
      - Through: 12/31
      - For: AllDays
      - Until: 06:00, 30.0
      - Until: 23:00, 26.0
      - Until: 24:00, 30.0

ScheduleTypeLimits:
  - Name: Temperature
    Lower Limit Value: -100.0
    Upper Limit Value: 200.0
    Numeric Type: Continuous

# Ideal Loads Air System (simplified HVAC)
ZoneHVAC:IdealLoadsAirSystem:
  - Name: Zone_F1_IdealAir
    Zone Name: Zone_F1
    Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Temperature: 50.0
    Minimum Heating Supply Air Temperature: 10.0
    Maximum Cooling Supply Air Temperature: 15.0
    Minimum Cooling Supply Air Temperature: 10.0
    Heating Availability Schedule Name: HVAC Operation
    Cooling Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Humidity Ratio: 0.015
    Minimum Heating Supply Air Humidity Ratio: 0.0
    Maximum Cooling Supply Air Humidity Ratio: 0.008
    Minimum Cooling Supply Air Humidity Ratio: 0.0
    Heating Limit: NoLimit
    Maximum Heating Air Flow Rate: 
    Maximum Sensible Heating Capacity: 
    Cooling Limit: NoLimit
    Maximum Cooling Air Flow Rate: 
    Maximum Total Cooling Capacity: 
    Dehumidification Control Type: None
    Humidification Control Type: None
    Cooling Supply Air Path Name: 
    Heating Supply Air Path Name: 
    Outdoor Air Path Name: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Outdoor Air Flow Rate per Person: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Heat Recovery Type: None
    Sensible Heat Recovery Effectiveness: 0.7
    Latent Heat Recovery Effectiveness: 0.65
    Time of Day Heat Recovery Availability Schedule Name: 
    Economizer Control Type: NoEconomizer
    Economizer Control Action Type: 
    Economizer Maximum Limit Dry-Bulb Temperature: 
    Economizer Maximum Limit Enthalpy: 
    Economizer Maximum Limit Dewpoint Temperature: 
    Economizer Minimum Limit Dry-Bulb Temperature: 

  - Name: Zone_F2_IdealAir
    Zone Name: Zone_F2
    Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Temperature: 50.0
    Minimum Heating Supply Air Temperature: 10.0
    Maximum Cooling Supply Air Temperature: 15.0
    Minimum Cooling Supply Air Temperature: 10.0
    Heating Availability Schedule Name: HVAC Operation
    Cooling Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Humidity Ratio: 0.015
    Minimum Heating Supply Air Humidity Ratio: 0.0
    Maximum Cooling Supply Air Humidity Ratio: 0.008
    Minimum Cooling Supply Air Humidity Ratio: 0.0
    Heating Limit: NoLimit
    Maximum Heating Air Flow Rate: 
    Maximum Sensible Heating Capacity: 
    Cooling Limit: NoLimit
    Maximum Cooling Air Flow Rate: 
    Maximum Total Cooling Capacity: 
    Dehumidification Control Type: None
    Humidification Control Type: None
    Cooling Supply Air Path Name: 
    Heating Supply Air Path Name: 
    Outdoor Air Path Name: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Outdoor Air Flow Rate per Person: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Heat Recovery Type: None
    Sensible Heat Recovery Effectiveness: 0.7
    Latent Heat Recovery Effectiveness: 0.65
    Time of Day Heat Recovery Availability Schedule Name: 
    Economizer Control Type: NoEconomizer
    Economizer Control Action Type: 
    Economizer Maximum Limit Dry-Bulb Temperature: 
    Economizer Maximum Limit Enthalpy: 
    Economizer Maximum Limit Dewpoint Temperature: 
    Economizer Minimum Limit Dry-Bulb Temperature: 

  - Name: Zone_F3_IdealAir
    Zone Name: Zone_F3
    Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Temperature: 50.0
    Minimum Heating Supply Air Temperature: 10.0
    Maximum Cooling Supply Air Temperature: 15.0
    Minimum Cooling Supply Air Temperature: 10.0
    Heating Availability Schedule Name: HVAC Operation
    Cooling Availability Schedule Name: HVAC Operation
    Maximum Heating Supply Air Humidity Ratio: 0.015
    Minimum Heating Supply Air Humidity Ratio: 0.0
    Maximum Cooling Supply Air Humidity Ratio: 0.008
    Minimum Cooling Supply Air Humidity Ratio: 0.0
    Heating Limit: NoLimit
    Maximum Heating Air Flow Rate: 
    Maximum Sensible Heating Capacity: 
    Cooling Limit: NoLimit
    Maximum Cooling Air Flow Rate: 
    Maximum Total Cooling Capacity: 
    Dehumidification Control Type: None
    Humidification Control Type: None
    Cooling Supply Air Path Name: 
    Heating Supply Air Path Name: 
    Outdoor Air Path Name: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Outdoor Air Flow Rate per Person: 
    Outdoor Air Flow Rate per Zone Floor Area: 
    Heat Recovery Type: None
    Sensible Heat Recovery Effectiveness: 0.7
    Latent Heat Recovery Effectiveness: 0.65
    Time of Day Heat Recovery Availability Schedule Name: 
    Economizer Control Type: NoEconomizer
    Economizer Control Action Type: 
    Economizer Maximum Limit Dry-Bulb Temperature: 
    Economizer Maximum Limit Enthalpy: 
    Economizer Maximum Limit Dewpoint Temperature: 
    Economizer Minimum Limit Dry-Bulb Temperature: 

# -----------------------------------------------------------------------------
# Output Control
# -----------------------------------------------------------------------------

OutputControl:ReportingTolerances:
  Column Tolerance: 0.00001

Output:Table:SummaryReports:
  Report Name 0: AllSummary

Output:Meter:
  - Name: Electricity:Facility
    Reporting Frequency: Hourly

Output:Meter:
  - Name: Gas:Facility
    Reporting Frequency: Hourly

Output:Meter:
  - Name: EnergyTransfer:Facility
    Reporting Frequency: Hourly

Output:Meter:
  - Name: Zone Air Temperature
    Reporting Frequency: Hourly

Output:Meter:
  - Name: Zone Air Relative Humidity
    Reporting Frequency: Hourly

# -----------------------------------------------------------------------------
# Simulation Control
# -----------------------------------------------------------------------------

SimulationControl:
  Do Zone Sizing Calculation: Yes
  Do System Sizing Calculation: Yes
  Do Plant Sizing Calculation: No
  Run Simulation for Sizing Periods: No
  Run Simulation for Weather File Run Periods: Yes

RunPeriod:
  - Name: Full Year
    Begin Month: 1
    Begin Day of Month: 1
    End Month: 12
    End Day of Month: 31
    Year: 2024
    Day of Week for Start Day: Monday
    Use Weather File Holidays and Special Days: Yes
    Use Weather File Daylight Saving Period: Yes
    Apply Weekend Holiday Rule: Yes
    Use Weather File Rain Indicators: Yes
    Use Weather File Snow Indicators: Yes

Timestep:
  Number of Timesteps per Hour: 4

# =============================================================================
# End of EnergyPlus Configuration
# =============================================================================
```

---

