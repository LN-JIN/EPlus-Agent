from eppy.modeleditor import IDF

from src.converters.base_converter import BaseConverter
from src.utils.logging import get_logger
from src.validator.data_model import BuildingSchema

logger = get_logger(__name__)


class BuildingConverter(BaseConverter):
    def __init__(self, idf: IDF):
        super().__init__(idf)

    def convert(self, data: dict) -> None:
        self.logger.info("Building Converter Starting...")

        building_data: dict = data.get("Building", {})

        try:
            validated_data = self.validate(building_data)
            self._add_to_idf(validated_data)
        except Exception as e:
            self.state["failed"] += 1
            self.logger.error(f"Error Convert Building Data: {e}")

    def _add_to_idf(self, val_data: dict) -> None:
        building_data: BuildingSchema = val_data["building_data"]

        try:
            if not self.idf.getobject("Building", name=building_data.name):
                self.idf.newidfobject(
                    "Building",
                    Name=building_data.name,
                    North_Axis=building_data.north_axis,
                    Terrain=building_data.terrain,
                    Loads_Convergence_Tolerance_Value=building_data.loads_convergence_tolerance_value,
                    Temperature_Convergence_Tolerance_Value=building_data.temperature_convergence_tolerance_value,
                    Solar_Distribution=building_data.solar_distribution,
                    Maximum_Number_of_Warmup_Days=building_data.maximum_number_of_warmup_days,
                    Minimum_Number_of_Warmup_Days=building_data.minimum_number_of_warmup_days,
                )
                self.state["success"] += 1
                self.logger.success(
                    f"Building object with name {building_data.name} added to IDF."
                )
            else:
                self.logger.warning(
                    f"Building object with name {building_data.name} already exists in IDF. Skipping addition."
                )
                self.state["skipped"] += 1
        except Exception as e:
            self.state["failed"] += 1
            self.logger.error(f"Error Adding Building to IDF: {e}")

    def validate(self, data: dict) -> dict:
        val_building_data = BuildingSchema.model_validate(data)
        return {
            "building_data": val_building_data,
        }
