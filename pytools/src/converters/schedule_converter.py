from typing import Any

from eppy.modeleditor import IDF

from src.converters.base_converter import BaseConverter
from src.utils.logging import get_logger
from src.validator.data_model import (
    ScheduleCollectionSchema,
    ScheduleCompactSchema,
    ScheduleTypeLimitsSchema,
)


class ScheduleConverter(BaseConverter):
    """
    Converts Schedule component definitions from YAML data into IDF objects.
    Handles ScheduleTypeLimits and Schedule:Compact.
    """

    def __init__(self, idf: IDF):
        super().__init__(idf)
        self.logger = get_logger(__name__)

    def convert(self, data: dict[str, Any]) -> None:
        self.logger.info("Schedule Converter Starting...")
        schedule_data = data.get("Schedule", {})
        if not schedule_data:
            self.logger.info("No Schedule data found in YAML.")
            return

        try:
            validated_data = self.validate(schedule_data)
        except Exception as e:
            self.state["failed"] += 1
            self.logger.error(f"Failed to validate Schedule data: {e}")
            return

        self._cross_validate_schedules(validated_data)

        for schedule_type_limits in validated_data.schedule_type_limits:
            self._add_to_idf(schedule_type_limits)

        for schedule_compact in validated_data.schedules:
            self._add_to_idf(schedule_compact)

    def _add_to_idf(self, val_data: Any) -> None:
        try:
            if isinstance(val_data, ScheduleTypeLimitsSchema):
                if not self.idf.getobject("ScheduleTypeLimits", val_data.name):
                    self.idf.newidfobject(
                        "ScheduleTypeLimits",
                        Name=val_data.name,
                        Lower_Limit_Value=val_data.lower_limit_value,
                        Upper_Limit_Value=val_data.upper_limit_value,
                        Numeric_Type=val_data.numeric_type,
                        Unit_Type=val_data.unit_type,
                    )
                    self.state["success"] += 1
                    self.logger.success(
                        f"ScheduleTypeLimits with name {val_data.name} added to IDF."
                    )
                else:
                    self.logger.warning(
                        f"ScheduleTypeLimits with name {val_data.name} already exists in IDF. Skipping addition."
                    )
                    self.state["skipped"] += 1
            elif isinstance(val_data, ScheduleCompactSchema):
                if not self.idf.getobject("Schedule:Compact", val_data.name):
                    schdule = self.idf.newidfobject(
                        "Schedule:Compact",
                        Name=val_data.name,
                        Schedule_Type_Limits_Name=val_data.schedule_type_limits_name,
                    )
                    for i, value in enumerate(val_data.data):
                        setattr(schdule, f"Field_{i + 1}", value)
                    self.state["success"] += 1
                    self.logger.success(
                        f"Schedule:Compact with name {val_data.name} added to IDF."
                    )
                else:
                    self.logger.warning(
                        f"Schedule:Compact with name {val_data.name} already exists in IDF. Skipping addition."
                    )
                    self.state["skipped"] += 1
            else:
                self.state["failed"] += 1
                raise ValueError(f"Unknown Schedule object type: {type(val_data)}")
        except Exception as e:
            self.state["failed"] += 1
            self.logger.error(f"Failed to add Schedule object: {e}")

    def _cross_validate_schedules(self, collection: ScheduleCollectionSchema) -> None:
        """Check that Schedule:Compact values are within their ScheduleTypeLimits bounds."""
        type_limits_map = {stl.name: stl for stl in collection.schedule_type_limits}
        for schedule in collection.schedules:
            stl = type_limits_map.get(schedule.schedule_type_limits_name)
            if stl is None:
                continue
            lower = stl.lower_limit_value
            upper = stl.upper_limit_value
            if lower == "" and upper == "":
                continue  # Any Number — no bounds
            for entry in schedule.data:
                if not entry.startswith("Until:"):
                    continue
                parts = entry.split(",")
                if len(parts) != 2:
                    continue
                try:
                    value = float(parts[1].strip())
                except ValueError:
                    continue
                if isinstance(lower, (int, float)) and value < lower:
                    raise ValueError(
                        f"Schedule '{schedule.name}': value {value} is below the lower limit "
                        f"{lower} of ScheduleTypeLimits '{stl.name}'. "
                        f"Use a ScheduleTypeLimits with a wider range (e.g. 'Any Number')."
                    )
                if isinstance(upper, (int, float)) and value > upper:
                    raise ValueError(
                        f"Schedule '{schedule.name}': value {value} exceeds the upper limit "
                        f"{upper} of ScheduleTypeLimits '{stl.name}'. "
                        f"Use a ScheduleTypeLimits with a wider range (e.g. 'Any Number')."
                    )

    def validate(self, data: dict[str, Any]) -> Any:
        return ScheduleCollectionSchema.model_validate(data)
