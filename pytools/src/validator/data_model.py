from abc import abstractmethod
from collections import defaultdict
from io import StringIO
from pathlib import Path
from typing import Any, ClassVar, cast

import numpy as np
from dateutil.parser import parse
from eppy.modeleditor import IDF
from pydantic import (
    BaseModel,
    ConfigDict,
    Field,
    SerializationInfo,
    ValidationInfo,
    field_serializer,
    field_validator,
    model_validator,
)
from scipy.spatial import Delaunay

from src.utils.logging import get_logger

logger = get_logger(__name__)


class IDDField:
    def __init__(self, data: list[dict] | dict):
        if isinstance(data, list):
            for obj in data:
                if isinstance(obj, list):
                    if len(obj) > 0 and isinstance(obj[0], dict):
                        obj_name = obj[0].get("idfobj", None)
                    else:
                        continue
                    if obj_name:
                        obj_name = self._clean_key(obj_name)
                        setattr(self, obj_name, IDDField(obj[1:]))
                elif isinstance(obj, dict):
                    field_name = obj.get("field", None)
                    if field_name:
                        if (
                            isinstance(field_name, (list, tuple))
                            and len(field_name) > 0
                        ):
                            field_name = self._clean_key(field_name[0])
                        elif isinstance(field_name, str):
                            field_name = self._clean_key(field_name)
                        else:
                            continue
                        setattr(self, field_name, IDDField(obj))
        elif isinstance(data, dict):
            for key, value in data.items():
                key = self._clean_key(key)
                if isinstance(value, list) and len(value) == 1:
                    value = value[0]
                setattr(self, key, value)

    def _clean_key(self, key: str) -> str:
        for i in [" ", "-", "/", ":"]:
            key = key.replace(i, "_")
        return key

    def __getattr__(self, name: str) -> "IDDField":
        raise AttributeError(
            f"'{self.__class__.__name__}' object has no attribute '{name}'"
        )


class BaseSchema(BaseModel):
    model_config = ConfigDict(
        from_attributes=True,  # 支持从对象创建模型
        validate_assignment=True,  # 赋值时验证
        arbitrary_types_allowed=True,  # 允许任意类型
        str_strip_whitespace=True,  # 自动去除字符串空格
        use_enum_values=True,  # 使用枚举值
        populate_by_name=True,  # 允许通过字段名填充
        extra="allow",  # 允许额外字段
    )

    _idf: IDF | None = None
    _idf_field: IDDField = IDDField({})

    @classmethod
    def set_idf(cls, idd_file: Path | str, idf_path: Path | None = None) -> None:
        IDF.setiddname(str(idd_file))
        if idf_path:
            cls._idf = IDF(str(idf_path))
        else:
            cls._idf = cls._create_blank_idf()
        cls._idf_field = cls._process_idf_field()

    @classmethod
    def _process_idf_field(cls) -> IDDField:
        if cls._idf is None:
            raise RuntimeError(
                "IDF has not been initialized. Call BaseSchema.set_idf(...) before using _process_idf_field."
            )
        _idd_info = cast(list[dict], cls._idf.idd_info)
        idd_field = IDDField(_idd_info)
        return idd_field

    @staticmethod
    def _create_blank_idf() -> IDF:
        idf_text = ""
        fhandle = StringIO(idf_text)
        return IDF(fhandle)

    @staticmethod
    def validate_choice_field(
        value: str, valid_choices: list | IDDField, field_name: str
    ) -> str:
        choice_mapping = {choice.lower(): choice for choice in valid_choices}  # type: ignore
        value_lower = value.lower()

        if value_lower not in choice_mapping:
            logger.error(
                f"{field_name} '{value}' is not a valid choice. Valid choices are: {valid_choices}."
            )
            raise ValueError(f"{field_name} must be one of {valid_choices}.")

        if value not in valid_choices:  # type: ignore
            logger.warning(
                f"{field_name} '{value}' is not in the standard casing. Using '{choice_mapping[value_lower]}' instead."
            )
        return choice_mapping[value_lower]

    @abstractmethod
    def to_yaml_dict(self) -> dict[str, Any]: ...

    @classmethod
    def get_idf(cls) -> IDF:
        if cls._idf is None:
            raise ValueError(
                "IDF is not set. Please set the IDF using BaseSchema.set_idf."
            )
        return cls._idf


class BuildingSchema(BaseSchema):
    name: str = Field(..., alias="Name", description="Building name")
    north_axis: float = Field(
        0.0, alias="North Axis", description="Building north axis in degrees"
    )
    terrain: str = Field("Suburbs", alias="Terrain", description="Terrain type")
    loads_convergence_tolerance_value: float = Field(
        0.04,
        alias="Loads Convergence Tolerance Value",
        description="Loads convergence tolerance value",
    )
    temperature_convergence_tolerance_value: float = Field(
        0.4,
        alias="Temperature Convergence Tolerance Value",
        description="Temperature convergence tolerance value",
    )
    solar_distribution: str = Field(
        "FullExterior", alias="Solar Distribution", description="Solar distribution"
    )
    maximum_number_of_warmup_days: int = Field(
        25,
        alias="Maximum Number of Warmup Days",
        description="Maximum number of warmup days",
    )
    minimum_number_of_warmup_days: int = Field(
        1,
        alias="Minimum Number of Warmup Days",
        description="Minimum number of warmup days",
    )

    @field_validator("name")
    def validate_name(cls, v):
        if not v:
            raise ValueError("Name must not be empty.")
        return v

    @field_validator("north_axis")
    def validate_north_axis(cls, v):
        if not (0 <= v < 360):
            raise ValueError("North Axis must be in [0, 360).")
        return v

    @field_validator("terrain")
    def validate_terrain(cls, v):
        valid_terrains = cls._idf_field.Building.Terrain.key
        if v not in valid_terrains:  # type: ignore[operator]
            raise ValueError(f"Terrain must be one of {valid_terrains}.")
        return v

    @field_validator(
        "loads_convergence_tolerance_value", "temperature_convergence_tolerance_value"
    )
    def validate_positive(cls, v):
        if v <= 0:
            raise ValueError("Value must be positive.")
        return v

    @field_validator("solar_distribution")
    def validate_solar_distribution(cls, v):
        valid_distribution = {
            "FullExterior",
            "MinimalShadowing",
            "FullInteriorAndExterior",
            "FullExteriorWithReflections",
            "FullInteriorAndExteriorWithReflections",
        }
        if v not in valid_distribution:
            raise ValueError(f"Solar Distribution must be one of {valid_distribution}.")
        return v

    @field_validator("maximum_number_of_warmup_days")
    def validate_max_warmup_days(cls, v):
        if v < 1:
            raise ValueError("Maximum Number of Warmup Days must be at least 1.")
        return v

    @field_validator("minimum_number_of_warmup_days")
    def validate_min_warmup_days(cls, v):
        if v < 1:
            raise ValueError("Minimum Number of Warmup Days must be at least 1 (EnergyPlus requirement).")
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Building": self.model_dump(by_alias=True)}


class VersionSchema(BaseSchema):
    version: str | tuple | list = Field(
        ..., alias="Version Identifier", description="Version identifier"
    )

    @field_validator("version")
    def validate_version(cls, v):
        if not v:
            raise ValueError("Version Identifier must not be empty.")
        if isinstance(v, (list, tuple)):
            return ".".join([str(i) for i in v])
        if isinstance(v, str):
            return v
        raise ValueError(
            "Version Identifier must be a string or a tuple/list of integers."
        )

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Version": self.model_dump(by_alias=True)}


class ZoneSchema(BaseSchema):
    name: str = Field(..., alias="Name", description="Zone name")
    direction_of_relative_north: float | None = Field(
        0.0,
        alias="Direction of Relative North",
        description="Direction of relative north in degrees",
    )
    x_origin: float = Field(0.0, alias="X Origin", description="X origin coordinate")
    y_origin: float = Field(0.0, alias="Y Origin", description="Y origin coordinate")
    z_origin: float = Field(0.0, alias="Z Origin", description="Z origin coordinate")
    type: int = Field(
        1, alias="Type", description="Zone type is currently unused in EnergyPlus"
    )
    multiplier: int = Field(1, alias="Multiplier", description="Zone multiplier", ge=1)
    ceiling_height: str | float = Field(
        "autocalculate",
        alias="Ceiling Height",
        description="Ceiling height in meters or 'autocalculate'",
    )
    volume: str | float = Field(
        "autocalculate",
        alias="Volume",
        description="Zone volume in cubic meters or 'autocalculate'",
    )
    floor_area: str | float = Field(
        "autocalculate",
        alias="Floor Area",
        description="Zone floor area in square meters or 'autocalculate'",
    )
    zone_inside_convection_algorithm: str = Field(
        "TARP",
        alias="Zone Inside Convection Algorithm",
        description="Zone inside convection algorithm",
    )
    zone_outside_convection_algorithm: str = Field(
        "DOE-2",
        alias="Zone Outside Convection Algorithm",
        description="Zone outside convection algorithm",
    )
    part_of_total_floor_area: str = Field(
        "Yes", alias="Part of Total Floor Area", description="Part of total floor area"
    )

    @field_validator("name")
    def validate_name(cls, v):
        if not v:
            raise ValueError("Name must not be empty.")
        return v

    @field_validator("direction_of_relative_north")
    def validate_direction_of_relative_north(cls, v):
        if v is not None and not (0 <= v < 360):
            raise ValueError("Direction of Relative North must be in [0, 360).")
        elif v is None:
            return 0.0
        return v

    @field_validator("x_origin", "y_origin", "z_origin")
    def validate_origin(cls, v):
        if not isinstance(v, (int, float)):
            raise ValueError("Origin coordinates must be numeric.")
        return v

    @field_validator("type")
    def validate_type(cls, v):
        if v < 0:
            raise ValueError("Zone Type must be non-negative.")
        return v

    @field_validator("multiplier")
    def validate_multiplier(cls, v):
        if v < 1:
            raise ValueError("Multiplier must be at least 1.")
        return v

    @field_validator("ceiling_height", "volume", "floor_area")
    def validate_autocalculate_or_positive(cls, v):
        if isinstance(v, str) and v.lower() == "autocalculate":
            return "autocalculate"
        try:
            fv = float(v)
            if fv <= 0:
                raise ValueError("Value must be positive or 'autocalculate'.")
            return fv
        except (TypeError, ValueError) as e:
            raise ValueError("Value must be a number or 'autocalculate'.") from e

    @field_validator("zone_inside_convection_algorithm")
    def validate_zone_inside_convection_algorithm(cls, v):
        valid_algorithms = {
            "Simple",
            "TARP",
            "CeilingDiffuser",
            "AdaptiveConvectionAlgorithm",
            "TrombeWall",
            "ASTMC1340",
        }
        if v not in valid_algorithms:
            raise ValueError(
                f"Zone Inside Convection Algorithm must be one of {valid_algorithms}."
            )
        return v

    @field_validator("zone_outside_convection_algorithm")
    def validate_zone_outside_convection_algorithm(cls, v):
        valid_algorithms = {
            "Simple",
            "TARP",
            "DOE-2",
            "MoWiTT",
            "AdaptiveConvectionAlgorithm",
        }
        if v not in valid_algorithms:
            raise ValueError(
                f"Zone Outside Convection Algorithm must be one of {valid_algorithms}."
            )
        return v

    @field_validator("part_of_total_floor_area")
    def validate_part_of_total_floor_area(cls, v):
        valid_options = {"Yes", "No"}
        if v not in valid_options:
            raise ValueError(
                f"Part of Total Floor Area must be one of {valid_options}."
            )
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Zone": self.model_dump(by_alias=True)}


class SurfaceSchema(BaseSchema):
    name: str = Field(..., alias="Name", description="Surface name")
    surface_type: str = Field(..., alias="Surface Type", description="Type of surface")
    construction_name: str = Field(
        ..., alias="Construction Name", description="Name of the construction"
    )
    zone_name: str = Field(
        ..., alias="Zone Name", description="Name of the associated zone"
    )
    space_name: str | None = Field(
        None, alias="Space Name", description="Name of the associated space"
    )
    outside_boundary_condition: str = Field(
        ...,
        alias="Outside Boundary Condition",
        description="Outside boundary condition",
    )
    outside_boundary_condition_object: str | None = Field(
        None,
        alias="Outside Boundary Condition Object",
        description="Outside boundary condition object",
    )
    sun_exposure: str = Field("NoSun", alias="Sun Exposure", description="Sun exposure")
    wind_exposure: str = Field(
        "NoWind", alias="Wind Exposure", description="Wind exposure"
    )
    view_factor_to_ground: str | float = Field(
        "autocalculate",
        alias="View Factor to Ground",
        description="View factor to ground or 'autocalculate'",
    )
    vertices: np.ndarray = Field(
        ..., alias="Vertices", description="List of vertices defining the surface"
    )

    @field_validator("name", "construction_name", "zone_name")
    def validate_non_empty(cls, v):
        if not v:
            raise ValueError("This field must not be empty.")
        return v

    @field_validator("surface_type")
    def validate_surface_type(cls, v):
        valid_types = cls._idf_field.BuildingSurface_Detailed.Surface_Type.key
        if v not in valid_types:  # type: ignore[operator]
            raise ValueError(f"Surface Type must be one of {valid_types}.")
        return v

    @field_validator("outside_boundary_condition")
    def validate_outside_boundary_condition(cls, v):
        valid_conditions = (
            cls._idf_field.BuildingSurface_Detailed.Outside_Boundary_Condition.key
        )
        if v not in valid_conditions:  # type: ignore[operator]
            raise ValueError(
                f"Outside Boundary Condition must be one of {valid_conditions}."
            )
        return v

    @field_validator("sun_exposure")
    def validate_sun_exposure(cls, v):
        valid_exposures = cls._idf_field.BuildingSurface_Detailed.Sun_Exposure.key
        if v not in valid_exposures:  # type: ignore[operator]
            raise ValueError(f"Sun Exposure must be one of {valid_exposures}.")
        return v

    @field_validator("wind_exposure")
    def validate_wind_exposure(cls, v):
        valid_exposures = cls._idf_field.BuildingSurface_Detailed.Wind_Exposure.key
        if v not in valid_exposures:  # type: ignore[operator]
            raise ValueError(f"Wind Exposure must be one of {valid_exposures}.")
        return v

    @field_validator("view_factor_to_ground")
    def validate_view_factor_to_ground(cls, v):
        if isinstance(v, str) and v.lower() == "autocalculate":
            return "autocalculate"
        try:
            fv = float(v)
            if not (0.0 <= fv <= 1.0):
                raise ValueError("View Factor to Ground must be between 0.0 and 1.0.")
            return fv
        except (TypeError, ValueError) as e:
            raise ValueError(
                "View Factor to Ground must be a number between 0.0 and 1.0 or 'autocalculate'."
            ) from e

    @field_validator("vertices", mode="before")
    def validate_vertices(cls, v):
        if isinstance(v, np.ndarray):
            return v
        tolerance = 1e-10
        if len(v) < 3:
            raise ValueError(
                f"The surface must have at least 3 vertices. current has {len(v)}"
            )
        pts = np.array([[pt["X"], pt["Y"], pt["Z"]] for pt in v])
        diff = pts[:, np.newaxis, :] - pts[np.newaxis, :, :]
        distances = np.linalg.norm(diff, axis=2)
        np.fill_diagonal(distances, np.inf)

        mask = distances < tolerance
        if np.any(mask):
            for pt1, pt2 in np.argwhere(mask):
                logger.error(f"Vertices {v[pt1]} and {v[pt2]} are too close.")
            raise ValueError("Some vertices are too close to each other.")
        return pts

    @field_serializer("vertices")
    def serialize_vertices(self, v: np.ndarray, _info: SerializationInfo) -> list[dict]:
        return [{"X": float(pt[0]), "Y": float(pt[1]), "Z": float(pt[2])} for pt in v]

    @model_validator(mode="after")
    def validate_boundary_condition_object(self):
        needs_obj = {"Surface", "OtherSideCoefficients", "OtherSideConditionsModel"}
        if (
            self.outside_boundary_condition in needs_obj
            and not self.outside_boundary_condition_object
        ):
            raise ValueError(
                f"Outside Boundary Condition Object is required when "
                f"Outside Boundary Condition is '{self.outside_boundary_condition}'."
            )
        return self

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"BuildingSurface:Detailed": self.model_dump(by_alias=True)}


class SimulationControlSchema(BaseSchema):
    do_zone_sizing_calculation: str | bool = Field(
        "No", alias="Do Zone Sizing Calculation"
    )
    do_system_sizing_calculation: str | bool = Field(
        "No", alias="Do System Sizing Calculation"
    )
    do_plant_sizing_calculation: str | bool = Field(
        "No", alias="Do Plant Sizing Calculation"
    )
    run_simulation_for_sizing_periods: str | bool = Field(
        "No", alias="Run Simulation for Sizing Periods"
    )
    run_simulation_for_weather_file_run_periods: str | bool = Field(
        "Yes", alias="Run Simulation for Weather File Run Periods"
    )
    do_hvac_sizing_simulation_for_sizing_periods: str | bool | None = Field(
        "Yes", alias="Do HVAC Sizing Simulation for Sizing Periods"
    )
    maximum_number_of_hvac_sizing_simulation_passes: int | None = Field(
        1, alias="Maximum Number of HVAC Sizing Simulation Passes"
    )

    @field_validator(
        "do_zone_sizing_calculation",
        "do_system_sizing_calculation",
        "do_plant_sizing_calculation",
        "run_simulation_for_sizing_periods",
        "run_simulation_for_weather_file_run_periods",
        "do_hvac_sizing_simulation_for_sizing_periods",
        mode="before",
    )
    def convert_bool_to_yes_no(cls, v):
        if isinstance(v, bool):
            return "Yes" if v else "No"
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"SimulationControl": self.model_dump(by_alias=True)}


class TimestepSchema(BaseSchema):
    number_of_timesteps_per_hour: int = Field(4, alias="Number of Timesteps per Hour")

    @field_validator("number_of_timesteps_per_hour")
    def validate_timesteps(cls, v):
        if v < 1:
            raise ValueError("Number of Timesteps per Hour must be at least 1.")
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Timestep": self.model_dump(by_alias=True)}


class SiteLocationSchema(BaseSchema):
    name: str = Field(..., alias="Name")
    latitude: float = Field(..., alias="Latitude")
    longitude: float = Field(..., alias="Longitude")
    time_zone: float = Field(..., alias="Time Zone")
    elevation: float = Field(..., alias="Elevation")

    @field_validator("name")
    def validate_name(cls, v):
        if not v:
            raise ValueError("Name must not be empty.")
        return v

    @field_validator("latitude")
    def validate_latitude(cls, v):
        if not (-90 <= v <= 90):
            raise ValueError("Latitude must be between -90 and 90 degrees.")
        return v

    @field_validator("longitude")
    def validate_longitude(cls, v):
        if not (-180 <= v <= 180):
            raise ValueError("Longitude must be between -180 and 180 degrees.")
        return v

    @field_validator("time_zone")
    def validate_time_zone(cls, v):
        if not (-12 <= v <= 14):
            raise ValueError("Time Zone must be between -12 and 14 hours.")
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Site:Location": self.model_dump(by_alias=True)}


class RunPeriodSchema(BaseSchema):
    name: str = Field("Default Run Period", alias="Name")
    begin_month: int = Field(1, alias="Begin Month")
    begin_day_of_month: int = Field(1, alias="Begin Day of Month")
    begin_year: int | None = Field(None, alias="Begin Year")
    end_month: int = Field(12, alias="End Month")
    end_day_of_month: int = Field(31, alias="End Day of Month")
    end_year: int | None = Field(None, alias="End Year")
    day_of_week_for_start_day: str | None = Field(
        None, alias="Day of Week for Start Day"
    )
    use_weather_file_holidays_and_special_days: str | bool | None = Field(
        None, alias="Use Weather File Holidays and Special Days"
    )
    use_weather_file_daylight_saving_period: str | bool | None = Field(
        None, alias="Use Weather File Daylight Saving Period"
    )
    apply_weekend_holiday_rule: str | bool | None = Field(
        None, alias="Apply Weekend Holiday Rule"
    )
    use_weather_file_rain_indicators: str | bool | None = Field(
        None, alias="Use Weather File Rain Indicators"
    )
    use_weather_file_snow_indicators: str | bool | None = Field(
        None, alias="Use Weather File Snow Indicators"
    )

    @field_validator(
        "use_weather_file_holidays_and_special_days",
        "use_weather_file_daylight_saving_period",
        "apply_weekend_holiday_rule",
        "use_weather_file_rain_indicators",
        "use_weather_file_snow_indicators",
        mode="before",
    )
    def convert_bool_to_yes_no_runperiod(cls, v):
        if v is None:
            return None
        if isinstance(v, bool):
            return "Yes" if v else "No"
        return v

    @field_validator("begin_month", "end_month")
    def validate_month(cls, v):
        if not (1 <= v <= 12):
            raise ValueError("Month must be between 1 and 12.")
        return v

    @model_validator(mode="after")
    def validate_month_oder(self):
        if self.begin_month > self.end_month:
            raise ValueError("Begin Month must be less than or equal to End Month.")
        return self

    @field_validator("begin_day_of_month", "end_day_of_month")
    def validate_day(cls, v):
        if not (1 <= v <= 31):
            raise ValueError("Day of Month must be between 1 and 31.")
        return v

    @model_validator(mode="after")
    def validate_day_order(self):
        if (
            self.begin_month == self.end_month
            and self.begin_day_of_month > self.end_day_of_month
        ):
            raise ValueError(
                "Begin Day of Month must be less than or equal to End Day of Month when Begin Month equals End Month."
            )
        return self

    @field_validator("day_of_week_for_start_day")
    def validate_day_of_week(cls, v):
        valid_days = cls._idf_field.RunPeriod.Day_of_Week_for_Start_Day.key
        if v is not None and v not in valid_days:  # type: ignore[operator]
            raise ValueError(f"Day of Week for Start Day must be one of {valid_days}.")
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"RunPeriod": self.model_dump(by_alias=True)}


class GlobalGeometryRulesSchema(BaseSchema):
    starting_vertex_position: str = Field(
        "UpperLeftCorner", alias="Starting Vertex Position"
    )
    vertex_entry_direction: str = Field(
        "Counterclockwise", alias="Vertex Entry Direction"
    )
    coordinate_system: str = Field("World", alias="Coordinate System")

    @field_validator("starting_vertex_position")
    def validate_starting_vertex_position(cls, v):
        valid_positions = (
            cls._idf_field.GlobalGeometryRules.Starting_Vertex_Position.key
        )
        if v not in valid_positions:  # type: ignore[operator]
            raise ValueError(
                f"Starting Vertex Position must be one of {valid_positions}."
            )
        return v

    @field_validator("vertex_entry_direction")
    def validate_vertex_entry_direction(cls, v):
        valid_directions = cls._idf_field.GlobalGeometryRules.Vertex_Entry_Direction.key
        return cls.validate_choice_field(v, valid_directions, "Vertex Entry Direction")

    @field_validator("coordinate_system")
    def validate_coordinate_system(cls, v):
        valid_systems = cls._idf_field.GlobalGeometryRules.Coordinate_System.key
        return cls.validate_choice_field(v, valid_systems, "Coordinate System")

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"GlobalGeometryRules": self.model_dump(by_alias=True)}


class OutputVariableDictionarySchema(BaseSchema):
    key_field: str = Field("Regular", alias="Key Field")

    @field_validator("key_field")
    def validate_key_field(cls, v):
        valid_key_field = cls._idf_field.Output_VariableDictionary.Key_Field.key
        return cls.validate_choice_field(v, valid_key_field, "Key Field")

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Output:VariableDictionary": self.model_dump(by_alias=True)}


class OutputDiagnosticsSchema(BaseSchema):
    key_1: str = Field("DisplayExtraWarnings", alias="Key 1")

    @field_validator("key_1")
    def validate_key_1(cls, v):
        valid_key_1 = cls._idf_field.Output_Diagnostics.Key_1.key
        return cls.validate_choice_field(v, valid_key_1, "Key 1")

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Output:Diagnostics": self.model_dump(by_alias=True)}


class OutputTableSummaryReportsSchema(BaseSchema):
    report_1_name: str = Field("AllSummary", alias="Report 1 Name")

    @field_validator("report_1_name")
    def validate_report_1_name(cls, v):
        valid_report_names = (
            cls._idf_field.Output_Table_SummaryReports.Report_1_Name.key
        )
        return cls.validate_choice_field(v, valid_report_names, "Report 1 Name")

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Output:Table:SummaryReports": self.model_dump(by_alias=True)}


class OutputControlTableStyleSchema(BaseSchema):
    column_separator: str = Field("Comma", alias="Column Separator")
    unit_conversion: str = Field("None", alias="Unit Conversion")

    @field_validator("column_separator")
    def validate_column_separator(cls, v):
        valid_separators = cls._idf_field.OutputControl_Table_Style.Column_Separator.key
        return cls.validate_choice_field(v, valid_separators, "Column Separator")

    @field_validator("unit_conversion")
    def validate_unit_conversion(cls, v):
        valid_conversions = cls._idf_field.OutputControl_Table_Style.Unit_Conversion.key
        return cls.validate_choice_field(v, valid_conversions, "Unit Conversion")

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"OutputControl:Table:Style": self.model_dump(by_alias=True)}


class OutputVariableSchema(BaseSchema):
    key_value: str = Field("*", alias="Key Value")
    variable_name: str = Field("Zone Mean Air Temperature", alias="Variable Name")
    reporting_frequency: str = Field("Hourly", alias="Reporting Frequency")

    @field_validator("reporting_frequency")
    def validate_reporting_frequency(cls, v):
        valid_frequencies = cls._idf_field.Output_Variable.Reporting_Frequency.key
        return cls.validate_choice_field(v, valid_frequencies, "Reporting Frequency")

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Output:Variable": self.model_dump(by_alias=True)}


class MaterialSchema(BaseSchema):
    name: str = Field(..., alias="Name")
    type: str = Field(..., alias="Type")

    @field_validator("name")
    def validate_name(cls, v: str) -> str:
        if not v:
            raise ValueError("Material Name must not be empty.")
        return v

    @field_validator("type")
    def validate_type(cls, v: str) -> str:
        valid_types = ["Standard", "NoMass", "AirGap", "Glazing"]
        if v not in valid_types:
            raise ValueError(f"Type must be one of {valid_types}.")
        return v

    @model_validator(mode="after")
    def validate_material(
        self,
    ) -> "MaterialSchema":
        if isinstance(
            self,
            (
                StandardMaterialSchema,
                NoMassMaterialSchema,
                AirGapMaterialSchema,
                GlazingMaterialSchema,
            ),
        ):
            return self
        if self.type == "Standard":
            return StandardMaterialSchema(**self.model_dump())
        elif self.type == "NoMass":
            return NoMassMaterialSchema(**self.model_dump())
        elif self.type == "AirGap":
            return AirGapMaterialSchema(**self.model_dump())
        elif self.type == "Glazing":
            return GlazingMaterialSchema(**self.model_dump())
        else:
            raise ValueError(f"Invalid material type: {self.type}")

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Material": self.model_dump(by_alias=True)}


class StandardMaterialSchema(MaterialSchema):
    roughness: str = Field(..., alias="Roughness")
    thickness: float = Field(..., alias="Thickness", gt=0)
    conductivity: float = Field(..., alias="Conductivity", gt=0)
    density: float = Field(..., alias="Density", gt=0)
    specific_heat: float = Field(..., alias="Specific_Heat", gt=0)

    @field_validator("roughness")
    def validate_roughness(cls, v: str) -> str:
        valid_choices = [
            "VeryRough",
            "Rough",
            "MediumRough",
            "MediumSmooth",
            "Smooth",
            "VerySmooth",
        ]
        if v not in valid_choices:
            raise ValueError(f"Roughness must be one of {valid_choices}.")
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"StandardMaterial": self.model_dump(by_alias=True)}


class NoMassMaterialSchema(MaterialSchema):
    roughness: str = Field(..., alias="Roughness")
    thermal_resistance: float = Field(..., alias="Thermal_Resistance", gt=0)

    @field_validator("roughness")
    def validate_roughness(cls, v: str) -> str:
        valid_choices = [
            "VeryRough",
            "Rough",
            "MediumRough",
            "MediumSmooth",
            "Smooth",
            "VerySmooth",
        ]
        if v not in valid_choices:
            raise ValueError(f"Roughness must be one of {valid_choices}.")
        return v


class AirGapMaterialSchema(MaterialSchema):
    thermal_resistance: float = Field(..., alias="Thermal_Resistance", gt=0)

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"AirGapMaterial": self.model_dump(by_alias=True)}


class GlazingMaterialSchema(MaterialSchema):
    u_factor: float = Field(..., alias="U-Factor", gt=0)
    solar_heat_gain_coefficient: float = Field(
        ..., alias="Solar_Heat_Gain_Coefficient", gt=0
    )
    visible_transmittance: float | None = Field(
        None, alias="Visible_Transmittance", ge=0, le=1
    )

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"GlazingMaterial": self.model_dump(by_alias=True)}


class ConstructionSchema(BaseSchema):
    name: str = Field(..., alias="Name")
    layers: list[str] = Field(..., alias="Layers", min_length=1)

    @field_validator("name")
    def validate_name(cls, v: str) -> str:
        if not v:
            raise ValueError("Construction Name must not be empty.")
        return v

    @field_validator("layers")
    def validate_layers(cls, v: list[str]) -> list[str]:
        if not all(isinstance(layer, str) and layer for layer in v):
            raise ValueError("All items in Layers must be non-empty strings.")
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Construction": self.model_dump(by_alias=True)}


class FenestrationSurfaceSchema(BaseSchema):
    name: str = Field(..., alias="Name", description="Fenestration surface name")
    surface_type: str = Field(..., alias="Surface Type", description="Type of surface")
    construction_name: str = Field(
        ..., alias="Construction Name", description="Name of the construction"
    )
    building_surface_name: str = Field(
        ..., alias="Building Surface Name", description="Name of the building surface"
    )
    outside_boundary_condition_object: str | None = Field(
        None,
        alias="Outside Boundary Condition Object",
        description="Outside boundary condition object",
    )
    frame_and_divider_name: str | None = Field(
        None, alias="Frame and Divider Name", description="Frame and divider name"
    )
    multiplier: int = Field(
        1, alias="Multiplier", description="Surface multiplier", ge=1
    )
    view_factor_to_ground: str | float = Field(
        "autocalculate",
        alias="View Factor to Ground",
        description="View factor to ground or 'autocalculate'",
    )
    Number_of_Vertices: str | int = Field(
        ...,
        alias="Number of Vertices",
        description="Number of vertices defining the surface",
    )
    vertices: np.ndarray = Field(
        ..., alias="Vertices", description="List of vertices defining the surface"
    )

    @field_validator("name", "construction_name", "building_surface_name")
    def validate_non_empty(cls, v):
        if not v:
            raise ValueError("This field must not be empty.")
        return v

    @field_validator("surface_type")
    def validate_surface_type(cls, v):
        valid_types = cls._idf_field.FenestrationSurface_Detailed.Surface_Type.key
        if v not in valid_types:  # type: ignore[operator]
            raise ValueError(f"Surface Type must be one of {valid_types}.")
        return v

    @field_validator("multiplier")
    def validate_multiplier(cls, v):
        if v < 1:
            raise ValueError("Multiplier must be at least 1.")
        return v

    @field_validator("vertices", mode="before")
    def validate_vertices(cls, v):
        if isinstance(v, np.ndarray):
            return v
        tolerance = 1e-10
        if len(v) < 3:
            raise ValueError(
                f"The surface must have at least 3 vertices. current has {len(v)}"
            )
        pts = np.array([[pt["X"], pt["Y"], pt["Z"]] for pt in v])
        diff = pts[:, np.newaxis, :] - pts[np.newaxis, :, :]
        distances = np.linalg.norm(diff, axis=2)
        np.fill_diagonal(distances, np.inf)

        mask = distances < tolerance
        if np.any(mask):
            for pt1, pt2 in np.argwhere(mask):
                raise ValueError(f"Vertices {v[pt1]} and {v[pt2]} are too close.")
        return pts

    @field_serializer("vertices")
    def serialize_vertices(self, v: np.ndarray, _info: SerializationInfo) -> list[dict]:
        return [{"X": float(pt[0]), "Y": float(pt[1]), "Z": float(pt[2])} for pt in v]

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"FenestrationSurface:Detailed": self.model_dump(by_alias=True)}


class GeometrySchema(BaseSchema):
    surfaces: list[SurfaceSchema] = Field(
        default_factory=list,
        alias="BuildingSurface:Detailed",
        description="List of building surfaces",
    )
    fenestrationsurfaces: list[FenestrationSurfaceSchema] = Field(
        default_factory=list,
        alias="FenestrationSurface:Detailed",
        description="List of fenestration surfaces",
    )
    _interior_points: np.ndarray = np.array([])
    _surface_to_normal_vector: ClassVar[dict[str, np.ndarray]] = {}

    @model_validator(mode="before")
    def validate_surfaces(cls, v):
        if "surfaces" not in v:
            return v
        result = defaultdict(list)
        surface_names = {surface["Name"] for surface in v.get("surfaces", [])}
        if len(surface_names) != len(v.get("surfaces", [])):
            raise ValueError("Surface names must be unique.")
        for surface in v.get("surfaces", []):
            result["surfaces"].append(SurfaceSchema.model_validate(surface))
        return result

    @model_validator(mode="before")
    def validate_fenestrationsurfaces(cls, v):
        if "fenestrationsurfaces" not in v:
            return v
        result = defaultdict(list)
        for surface in v.get("fenestrationsurfaces", []):
            result["fenestrationsurfaces"].append(
                FenestrationSurfaceSchema.model_validate(surface)
            )
        return result

    @field_validator("surfaces")
    def validate_geometry_closure(cls, v):
        # TODO: Consider the use of trimesh to implement a concave polygon triangularization closure check
        points = np.vstack([surface.vertices for surface in v]).round(8)
        unique_points, counts = np.unique(points, axis=0, return_counts=True)
        unclosure_indices = np.argwhere(counts < 3)
        if len(unclosure_indices) > 0:
            for idx in unclosure_indices:
                point = unique_points[idx]
                logger.error(f"Point {point} is not properly closed in the geometry.")
            raise ValueError(
                "Geometry closure validation failed. Some points are not properly closed."
            )
        return v

    @model_validator(mode="after")
    def validate_points_sorting(self):
        if not self.surfaces and not self.fenestrationsurfaces:
            return self
        if np.any(self._interior_points):
            interior_points = self._interior_points
        else:
            interior_points = np.array([])
        for surface in self.surfaces:
            if surface.surface_type == "Floor":
                interior_points = self._get_interior_points(surface)
                if not np.any(self._interior_points):
                    GeometrySchema._interior_points = interior_points
                surface.vertices = self._sort_vertices_clockwise(
                    surface, np.array([0, 0, -1])
                )
            elif surface.surface_type == "Roof" or surface.surface_type == "Ceiling":
                surface.vertices = self._sort_vertices_clockwise(
                    surface, np.array([0, 0, 1])
                )
        for surface in self.surfaces:
            if surface.surface_type not in {"Floor", "Roof", "Ceiling"}:
                if len(interior_points) == 0:
                    logger.error(
                        f"Cannot compute normal vector for surface {surface.name} without floor surfaces for reference."
                    )
                    raise ValueError(
                        "At least one Floor surface is required to validate other surface types."
                    )
                normal_vector = self._get_normal_vector(
                    surface.vertices, interior_points, surface.name
                )
                surface.vertices = self._sort_vertices_clockwise(surface, normal_vector)
        for surface in self.fenestrationsurfaces:
            normal_vector = self._get_normal_vector(
                surface.vertices, interior_points, surface.building_surface_name
            )
            surface.vertices = self._sort_vertices_clockwise(surface, normal_vector)
        return self

    def _sort_vertices_clockwise(
        self,
        surface: SurfaceSchema | FenestrationSurfaceSchema,
        normal_vector: np.ndarray,
    ):
        points = surface.vertices
        normal = normal_vector / np.linalg.norm(normal_vector)
        centroid = np.mean(points, axis=0)

        def compare_points(idx1, idx2):
            v1 = points[idx1] - centroid
            v2 = points[idx2] - centroid

            cross = np.cross(v1, v2)

            sign = np.dot(cross, normal)

            if sign > 1e-10:
                return -1
            elif sign < -1e-10:
                return 1
            else:
                d1 = np.linalg.norm(v1)
                d2 = np.linalg.norm(v2)
                return -1 if d1 < d2 else 1

        from functools import cmp_to_key

        sorted_indices = sorted(range(len(points)), key=cmp_to_key(compare_points))
        points = points[sorted_indices]
        top_left_index = self._get_top_left_corner_from_normal(points, normal_vector)

        return np.roll(points, -top_left_index, axis=0)

    def _get_interior_points(self, surface: SurfaceSchema) -> np.ndarray:
        interior_points = []
        if isinstance(surface.vertices, np.ndarray):
            try:
                tri = Delaunay(surface.vertices[:, :-1])
            except Exception as e:
                logger.exception(
                    f"Failed to perform Delaunay triangulation on surface {surface.name}: {e}"
                )
                raise ValueError(
                    f"Delaunay triangulation failed for surface {surface.name}."
                ) from e
        for simplex in tri.simplices:
            triangle_vertices = surface.vertices[simplex]
            centroid = triangle_vertices.mean(axis=0)
            interior_points.append(centroid.tolist())
        return np.array(interior_points)

    def _get_top_left_corner_from_normal(self, points, normal_vector) -> np.ndarray:
        normal = normal_vector / np.linalg.norm(normal_vector)

        world_up = np.array([0, 0, 1])

        if abs(np.dot(normal, world_up)) > 0.99:
            if np.dot(normal, world_up) > 0:
                world_up = np.array([0, 1, 0])
            else:
                world_up = np.array([0, -1, 0])

        right = np.cross(world_up, normal)
        right /= np.linalg.norm(right)

        up = np.cross(normal, right)
        up /= np.linalg.norm(up)

        centroid = np.mean(points, axis=0)
        relative_points = points - centroid

        x_coords = np.dot(relative_points, right)
        y_coords = np.dot(relative_points, up)

        sort_keys = np.column_stack((-y_coords, x_coords))
        top_left_index = np.lexsort((sort_keys[:, 1], sort_keys[:, 0]))[0]

        return top_left_index

    def _get_normal_vector(
        self, points: np.ndarray, interior_points: np.ndarray, surface_name: str
    ) -> np.ndarray:
        if surface_name in self._surface_to_normal_vector:
            return self._surface_to_normal_vector[surface_name]

        centroid = np.mean(points, axis=0)
        distances = np.linalg.norm(interior_points - centroid, axis=1)
        interior_vector = interior_points[np.argmin(distances)] - centroid

        v1 = points[1] - points[0]
        v2 = points[2] - points[0]

        if np.dot(np.cross(v1, v2), interior_vector) < 0:
            normal_vector = np.cross(v1, v2)
        else:
            normal_vector = np.cross(v2, v1)

        normal_vector = normal_vector / np.linalg.norm(normal_vector)

        self._surface_to_normal_vector[surface_name] = normal_vector

        return normal_vector

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Geometry": self.model_dump(by_alias=True)}


class ScheduleTypeLimitsSchema(BaseSchema):
    name: str = Field(..., alias="Name")
    lower_limit_value: float | None | str = Field(default="", alias="Lower Limit Value")
    upper_limit_value: float | None | str = Field(default="", alias="Upper Limit Value")
    numeric_type: str | None = Field("CONTINUOUS", alias="Numeric Type")
    unit_type: str | None = Field("Dimensionless", alias="Unit Type")

    @field_validator("name")
    def validate_name(cls, v: str) -> str:
        if not v:
            raise ValueError("ScheduleTypeLimits 'Name' must not be empty.")
        return v

    @field_validator("lower_limit_value", "upper_limit_value")
    def validate_limit_value(cls, v: float | None | str) -> float | None | str:
        if v == "":
            return ""
        if isinstance(v, (int, float)):
            return v
        raise ValueError("Limit values must be a number or an empty string.")

    @field_validator("numeric_type")
    def validate_numeric_type(cls, v: str | None) -> str | None:
        if v is None:
            return None
        valid_choices = ["CONTINUOUS", "DISCRETE"]
        return cls.validate_choice_field(v, valid_choices, "Numeric Type")

    @model_validator(mode="after")
    def check_limits(self) -> "ScheduleTypeLimitsSchema":
        if isinstance(self.lower_limit_value, str) and isinstance(
            self.upper_limit_value, str
        ):
            return self
        if (
            isinstance(self.lower_limit_value, (int, float))
            and isinstance(self.upper_limit_value, (int, float))
            and self.lower_limit_value < self.upper_limit_value
        ):
            return self
        raise ValueError(
            f"Type Limits for {self.name} are not valid. Lower limit ({self.lower_limit_value}), upper limit ({self.upper_limit_value})."
        )

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"ScheduleTypeLimits": self.model_dump(by_alias=True)}


class ScheduleCompactSchema(BaseSchema):
    name: str = Field(..., alias="Name")
    schedule_type_limits_name: str = Field(..., alias="Schedule Type Limits Name")
    data: list = Field(..., alias="Data", min_length=1)

    @field_validator("name", "schedule_type_limits_name")
    def validate_non_empty(cls, v: str, info: "ValidationInfo") -> str:
        if not v:
            raise ValueError(f"Field '{info.field_name}' must not be empty.")
        return v

    @field_validator("data")
    def validate_data(cls, v: list[str]) -> list[str]:
        return cls._validate_through(v)

    @classmethod
    def _validate_through(cls, data: list) -> list[str]:
        result = []
        for i, item in enumerate(data):
            date = item["Through"]
            date = parse(date).strftime("%m/%d")
            day_data = cls._validate_for(item["Days"])
            if i == len(data) - 1 and date != "12/31":
                raise ValueError("Schedule data must end with Through: 12/31")
            result.append(f"Through: {date}")
            result.extend(day_data)
        return result

    @classmethod
    def _validate_for(cls, data: list) -> list[str]:
        VALID_DAY_TYPES = {
            "weekdays",
            "weekends",
            "holidays",
            "alldays",
            "summerdesignday",
            "winterdesignday",
            "sunday",
            "monday",
            "tuesday",
            "wednesday",
            "thursday",
            "friday",
            "saturday",
            "customday1",
            "customday2",
            "allotherdays",
        }
        CATCH_ALL_TYPES = {"alldays", "allotherdays"}
        result = []
        last_time_data: list[str] = []
        covered = set()
        for _, item in enumerate(data):
            day_type = item["For"]
            if day_type.lower() not in VALID_DAY_TYPES:
                raise ValueError(f"Invalid day type: {day_type}")
            time_data = cls._validate_until(item["Times"])
            result.append(f"For: {day_type}")
            result.extend(time_data)
            covered.add(day_type.lower())
            last_time_data = time_data

        if not covered & CATCH_ALL_TYPES:
            result.append("For: AllOtherDays")
            result.extend(last_time_data)
        return result

    @classmethod
    def _validate_until(cls, data: list) -> list[str]:
        result = []
        for i, item in enumerate(data):
            time = item["Until"]["Time"]
            value = float(item["Until"]["Value"])
            if i == len(data) - 1:
                if time != "24:00":
                    raise ValueError(f"Last time entry must be 24:00, but got {time}")
            else:
                time = parse(time).strftime("%H:%M")
            result.append(f"Until: {time}, {value}")
        return result

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Schedule:Compact": self.model_dump(by_alias=True)}


class ScheduleCollectionSchema(BaseSchema):
    schedule_type_limits: list[ScheduleTypeLimitsSchema] = Field(
        default_factory=list, alias="ScheduleTypeLimits"
    )
    schedules: list[ScheduleCompactSchema] = Field(
        default_factory=list, alias="Schedule:Compact"
    )

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"ScheduleCollection": self.model_dump(by_alias=True)}


class HVACTemplateThermostatSchema(BaseSchema):
    name: str = Field(..., alias="Name")
    heating_setpoint_schedule_name: str = Field(
        ..., alias="Heating Setpoint Schedule Name"
    )
    cooling_setpoint_schedule_name: str = Field(
        ..., alias="Cooling Setpoint Schedule Name"
    )

    @field_validator(
        "name", "heating_setpoint_schedule_name", "cooling_setpoint_schedule_name"
    )
    def validate_non_empty(cls, v: str, info: "ValidationInfo") -> str:
        if not v:
            raise ValueError(f"Field '{info.field_name}' must not be empty.")
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"HVACTemplate:Thermostat": self.model_dump(by_alias=True)}


class HVACTemplateZoneIdealLoadsAirSystemSchema(BaseSchema):
    zone_name: str = Field(..., alias="Zone Name")
    template_thermostat_name: str = Field(..., alias="Template Thermostat Name")
    system_availability_schedule_name: str | None = Field(
        None, alias="System Availability Schedule Name"
    )

    @field_validator("zone_name", "template_thermostat_name")
    def validate_non_empty(cls, v: str, info: "ValidationInfo") -> str:
        if not v:
            raise ValueError(f"Field '{info.field_name}' must not be empty.")
        return v

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"HVACTemplate:Zone:IdealLoadsAirSystem": self.model_dump(by_alias=True)}


class HVACSchema(BaseSchema):
    """
    A container schema for all HVAC related components,
    now based on HVACTemplate objects.
    """

    thermostats: list[HVACTemplateThermostatSchema] | list = Field(
        default_factory=list, alias="HVACTemplate:Thermostat"
    )
    ideal_loads_systems: list[HVACTemplateZoneIdealLoadsAirSystemSchema] | list = Field(
        default_factory=list, alias="HVACTemplate:Zone:IdealLoadsAirSystem"
    )

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"HVAC": self.model_dump(by_alias=True)}


class LightSchema(BaseSchema):
    name: str = Field(..., alias="Name")
    zone_or_zone_list_or_space_or_space_list_name: str = Field(
        ..., alias="Zone or ZoneList or Space or SpaceList Name"
    )
    schedule_name: str = Field(..., alias="Schedule Name")
    design_level_calculation_method: str = Field(
        default="LightingLevel", alias="Design Level Calculation Method"
    )
    lighting_level: float | None = Field(default=0.0, alias="Lighting Level", ge=0.0)
    watts_per_floor_area: float | None = Field(
        default=0.0, alias="Watts per Floor Area", ge=0.0
    )
    watts_per_person: float | None = Field(
        default=0.0, alias="Watts per Person", ge=0.0
    )
    return_air_fraction: float | None = Field(
        default=0.0, alias="Return Air Fraction", ge=0.0, le=1.0
    )
    fraction_radiant: float | None = Field(
        default=0.0, alias="Fraction Radiant", ge=0.0, le=1.0
    )
    fraction_visible: float | None = Field(
        default=0.0, alias="Fraction Visible", ge=0.0, le=1.0
    )
    fraction_replaceable: float | None = Field(
        default=1.0, alias="Fraction Replaceable", ge=0.0, le=1.0
    )
    end_use_subcategory: str | None = Field(
        default="General", alias="End Use Subcategory"
    )
    return_air_fraction_calculated_from_plenum_temperature: str | None = Field(
        default="No", alias="Return Air Fraction Calculated from Plenum Temperature"
    )
    return_air_fraction_function_of_plenum_temperature_coefficient_1: float | None = (
        Field(
            default=0.0,
            alias="Return Air Fraction Function of Plenum Temperature Coefficient 1",
            ge=0.0,
        )
    )
    return_air_fraction_function_of_plenum_temperature_coefficient_2: float | None = (
        Field(
            default=0.0,
            alias="Return Air Fraction Function of Plenum Temperature Coefficient 2",
            ge=0.0,
        )
    )
    return_air_heat_gain_node_name: str | None = Field(
        "", alias="Return Air Heat Gain Node Name"
    )
    exhaust_air_heat_gain_node_name: str | None = Field(
        "", alias="Exhaust Air Heat Gain Node Name"
    )

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"Light": self.model_dump(by_alias=True)}

    @field_validator("design_level_calculation_method")
    def validate_design_level_calculation_method(cls, v: str) -> str:
        valid_choices = cls._idf_field.Lights.Design_Level_Calculation_Method.key
        return cls.validate_choice_field(
            v,
            valid_choices,
            "Design Level Calculation Method",
        )

    @model_validator(mode="after")
    def validate_calculation_method(self):
        count = sum(
            bool(x)
            for x in [
                self.lighting_level,
                self.watts_per_floor_area,
                self.watts_per_person,
            ]
        )
        if count != 1:
            raise ValueError(
                "Exactly one of Lighting Level, Watts per Floor Area, or Watts per Person must be specified."
            )
        if (
            self.design_level_calculation_method == "LightingLevel"
            and self.lighting_level == 0.0
        ):
            raise ValueError(
                "Lighting Level must be specified when Design Level Calculation Method is LightingLevel."
            )
        if (
            self.design_level_calculation_method == "Watts/Area"
            and self.watts_per_floor_area == 0.0
        ):
            raise ValueError(
                "Watts per Floor Area must be specified when Design Level Calculation Method is Watts/Area."
            )
        if (
            self.design_level_calculation_method == "Watts/Person"
            and self.watts_per_person == 0.0
        ):
            raise ValueError(
                "Watts per Person must be specified when Design Level Calculation Method is Watts/Person."
            )
        return self


class PeopleSchema(BaseSchema):
    name: str = Field(..., alias="Name")
    zone_or_zonelist_or_space_or_spacelist_name: str = Field(
        ..., alias="Zone or ZoneList or Space or SpaceList Name"
    )
    number_of_people_schedule_name: str = Field(
        ..., alias="Number of People Schedule Name"
    )
    number_of_people_calculation_method: str = Field(
        default="People", alias="Number of People Calculation Method"
    )
    number_of_people: float | None = Field(
        default=0.0, ge=0.0, alias="Number of People"
    )
    people_per_floor_area: float | None = Field(
        default=0.0, ge=0.0, alias="People per Floor Area"
    )
    floor_area_per_person: float | None = Field(
        default=0.0, ge=0.0, alias="Floor Area per Person"
    )
    fraction_radiant: float | None = Field(
        default=0.3, ge=0.0, le=1.0, alias="Fraction Radiant"
    )
    sensible_heat_fraction: float | str | None = Field(
        default="Autocalculate", alias="Sensible Heat Fraction"
    )
    activity_level_schedule_name: str = Field(..., alias="Activity Level Schedule Name")
    carbon_dioxide_generation_rate: float | None = Field(
        default=3.82e-08, ge=0.0, le=3.82e-07, alias="Carbon Dioxide Generation Rate"
    )
    enable_ashrae_55_comfort_warnings: str | None = Field(
        default="No", alias="Enable ASHRAE 55 Comfort Warnings"
    )
    mean_radiant_temperature_calculation_type: str | None = Field(
        default="EnclosureAveraged", alias="Mean Radiant Temperature Calculation Type"
    )
    surface_name_angle_factor_list_name: str | None = Field(
        default="", alias="Surface Name Angle Factor List Name"
    )
    work_efficiency_schedule_name: str | None = Field(
        default="", alias="Work Efficiency Schedule Name"
    )
    clothing_insulation_calculation_method: str | None = Field(
        default="ClothingInsulationSchedule",
        alias="Clothing Insulation Calculation Method",
    )
    clothing_insulation_calculation_method_schedule_name: str | None = Field(
        default="", alias="Clothing Insulation Calculation Method Schedule Name"
    )
    clothing_insulation_schedule_name: str | None = Field(
        default="", alias="Clothing Insulation Schedule Name"
    )
    air_velocity_schedule_name: str | None = Field(
        default="", alias="Air Velocity Schedule Name"
    )
    thermal_comfort_model_1_type: str | None = Field(
        default="", alias="Thermal Comfort Model 1 Type"
    )
    thermal_comfort_model_2_type: str | None = Field(
        default="", alias="Thermal Comfort Model 2 Type"
    )
    thermal_comfort_model_3_type: str | None = Field(
        default="", alias="Thermal Comfort Model 3 Type"
    )
    thermal_comfort_model_4_type: str | None = Field(
        default="", alias="Thermal Comfort Model 4 Type"
    )
    thermal_comfort_model_5_type: str | None = Field(
        default="", alias="Thermal Comfort Model 5 Type"
    )
    thermal_comfort_model_6_type: str | None = Field(
        default="", alias="Thermal Comfort Model 6 Type"
    )
    thermal_comfort_model_7_type: str | None = Field(
        default="", alias="Thermal Comfort Model 7 Type"
    )
    ankle_level_air_velocity_schedule_name: str | None = Field(
        default="", alias="Ankle Level Air Velocity Schedule Name"
    )
    cold_stress_temperature_threshold: float | None = Field(
        default=15.56, alias="Cold Stress Temperature Threshold"
    )
    heat_stress_temperature_threshold: float | None = Field(
        default=30.0, alias="Heat Stress Temperature Threshold"
    )

    def to_yaml_dict(self) -> dict[str, Any]:
        return {"People": self.model_dump(by_alias=True)}

    @model_validator(mode="after")
    def validate_number_of_people(self):
        count = sum(
            bool(x)
            for x in [
                self.number_of_people,
                self.people_per_floor_area,
                self.floor_area_per_person,
            ]
        )
        if count > 1:
            raise ValueError(
                "Only one of Number of People, People per Floor Area, or Floor Area per Person must be specified."
            )
        if self.number_of_people_calculation_method == "People":
            if self.number_of_people is None:
                raise ValueError(
                    'number_of_people must be provided when calculation method is "People"'
                )
        elif self.number_of_people_calculation_method == "People/Area":
            if self.people_per_floor_area is None:
                raise ValueError(
                    'people_per_floor_area must be provided when calculation method is "People/Area"'
                )
        elif self.number_of_people_calculation_method == "Area/Person":
            if self.floor_area_per_person is None:
                raise ValueError(
                    'floor_area_per_person must be provided when calculation method is "Area/Person"'
                )
        else:
            raise ValueError("Invalid Number of People Calculation Method.")
        return self

    @field_validator("number_of_people_calculation_method")
    def validate_number_of_people_calculation_method(cls, v):
        valid_choices = cls._idf_field.People.Number_of_People_Calculation_Method.key
        return cls.validate_choice_field(
            v,
            valid_choices,
            "Number of People Calculation Method",
        )

    @field_validator("sensible_heat_fraction")
    def validate_sensible_heat_fraction(cls, v):
        if isinstance(v, str) and v.lower() == "autocalculate":
            return v
        if isinstance(v, (float, int)):
            v = float(v)
            if v < 0.0 or v > 1.0:
                raise ValueError("Sensible Heat Fraction must be between 0.0 and 1.0.")
            return v
        raise ValueError(
            "Sensible Heat Fraction must be a number between 0.0 and 1.0 or 'autocalculate'."
        )

    @field_validator("mean_radiant_temperature_calculation_type")
    def validate_mean_radiant_temperature_calculation_type(cls, v):
        valid_choices = (
            cls._idf_field.People.Mean_Radiant_Temperature_Calculation_Type.key
        )
        return cls.validate_choice_field(
            v,
            valid_choices,
            "Mean Radiant Temperature Calculation Type",
        )

    @field_validator("clothing_insulation_calculation_method")
    def validate_clothing_insulation_calculation_method(cls, v):
        valid_choices = cls._idf_field.People.Clothing_Insulation_Calculation_Method.key
        return cls.validate_choice_field(
            v,
            valid_choices,
            "Clothing Insulation Calculation Method",
        )

    @field_validator(
        "thermal_comfort_model_1_type",
        "thermal_comfort_model_2_type",
        "thermal_comfort_model_3_type",
        "thermal_comfort_model_4_type",
        "thermal_comfort_model_5_type",
        "thermal_comfort_model_6_type",
        "thermal_comfort_model_7_type",
    )
    def validate_thermal_comfort_model_type(cls, v):
        if v in (None, ""):
            return v
        valid_choices = cls._idf_field.People.Thermal_Comfort_Model_1_Type.key
        return cls.validate_choice_field(
            v,
            valid_choices,
            "Thermal Comfort Model Type",
        )
