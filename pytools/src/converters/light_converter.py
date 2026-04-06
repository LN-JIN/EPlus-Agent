from eppy.modeleditor import IDF

from src.converters.base_converter import BaseConverter
from src.validator.data_model import LightSchema


class LightConverter(BaseConverter):
    def __init__(self, idf: IDF):
        super().__init__(idf)

    def convert(self, data: dict):
        self.logger.info("Converting light data...")
        for light in data.get("Light", []):
            try:
                validated_light = self.validate(light)
                self._add_to_idf(validated_light)
            except Exception as e:
                self.state["failed"] += 1
                self.logger.error(f"Error converting light data: {e}")
                continue

    def _add_to_idf(self, val_data: LightSchema):
        if self.idf.getobject("Lights", name=val_data.name):
            self.logger.warning(
                f"Light with name {val_data.name} already exists in IDF. Skipping addition."
            )
            self.state["skipped"] += 1
            return
        self.idf.newidfobject(
            "Lights",
            Name=val_data.name,
            Zone_or_ZoneList_or_Space_or_SpaceList_Name=val_data.zone_or_zone_list_or_space_or_space_list_name,
            Schedule_Name=val_data.schedule_name,
            Design_Level_Calculation_Method=val_data.design_level_calculation_method,
            Lighting_Level=val_data.lighting_level,
            Watts_per_Floor_Area=val_data.watts_per_floor_area,
            Watts_per_Person=val_data.watts_per_person,
            Return_Air_Fraction=val_data.return_air_fraction,
            Fraction_Radiant=val_data.fraction_radiant,
            Fraction_Visible=val_data.fraction_visible,
            Fraction_Replaceable=val_data.fraction_replaceable,
            EndUse_Subcategory=val_data.end_use_subcategory,
            Return_Air_Fraction_Calculated_from_Plenum_Temperature=val_data.return_air_fraction_calculated_from_plenum_temperature,
            Return_Air_Fraction_Function_of_Plenum_Temperature_Coefficient_1=val_data.return_air_fraction_function_of_plenum_temperature_coefficient_1,
            Return_Air_Fraction_Function_of_Plenum_Temperature_Coefficient_2=val_data.return_air_fraction_function_of_plenum_temperature_coefficient_2,
            Return_Air_Heat_Gain_Node_Name=val_data.return_air_heat_gain_node_name,
            Exhaust_Air_Heat_Gain_Node_Name=val_data.exhaust_air_heat_gain_node_name,
        )
        self.state["success"] += 1
        self.logger.success(f"Light with name {val_data.name} added to IDF.")

    def validate(self, data: dict) -> LightSchema:
        return LightSchema.model_validate(data)
