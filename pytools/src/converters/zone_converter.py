from typing import Any

from eppy.modeleditor import IDF

from src.converters.base_converter import BaseConverter
from src.validator.data_model import ZoneSchema


class ZoneConverter(BaseConverter):
    def __init__(self, idf: IDF):
        super().__init__(idf)

    def convert(self, data: dict) -> None:
        self.logger.info("Converting zone data...")
        for zd in data.get("Zone", []):
            try:
                val_data = self.validate(zd)
                self._add_to_idf(val_data)
            except Exception as e:
                self.state["failed"] += 1
                self.logger.error(f"Error processing Zone: {e}", exc_info=True)
                continue

    def _add_to_idf(self, val_data: Any) -> None:
        if self.idf.getobject("Zone", name=val_data.name):
            self.logger.warning(
                f"Zone with name {val_data.name} already exists in IDF. Skipping addition."
            )
            self.state["skipped"] += 1
            return
        try:
            self.idf.newidfobject(
                "Zone",
                Name=val_data.name,
                Direction_of_Relative_North=val_data.direction_of_relative_north,
                X_Origin=val_data.x_origin,
                Y_Origin=val_data.y_origin,
                Z_Origin=val_data.z_origin,
                Type=val_data.type,
                Multiplier=val_data.multiplier,
                Ceiling_Height=val_data.ceiling_height,
                Volume=val_data.volume,
                Floor_Area=val_data.floor_area,
                Zone_Inside_Convection_Algorithm=val_data.zone_inside_convection_algorithm,
                Zone_Outside_Convection_Algorithm=val_data.zone_outside_convection_algorithm,
                Part_of_Total_Floor_Area=val_data.part_of_total_floor_area,
            )
            self.state["success"] += 1
            self.logger.success(f"Zone with name {val_data.name} added to IDF.")
        except Exception as e:
            self.state["failed"] += 1
            self.logger.error(f"Error Adding Zone Data to IDF: {e}", exc_info=True)

    def validate(self, data: dict) -> Any:
        val_data = ZoneSchema.model_validate(data)
        return val_data
