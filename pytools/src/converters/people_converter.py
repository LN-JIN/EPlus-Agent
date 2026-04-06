from eppy.modeleditor import IDF

from src.converters.base_converter import BaseConverter
from src.validator.data_model import PeopleSchema


class PeopleConverter(BaseConverter):
    def __init__(self, idf: IDF):
        super().__init__(idf)

    def convert(self, data: dict):
        self.logger.info("Converting people data...")
        for people in data.get("People", []):
            try:
                validated_people = self.validate(people)
                self._add_to_idf(validated_people)
            except Exception as e:
                self.state["failed"] += 1
                self.logger.error(f"Error converting people data: {e}")
                continue

    def _add_to_idf(self, val_data: PeopleSchema):
        if self.idf.getobject("People", name=val_data.name):
            self.logger.warning(
                f"People with name {val_data.name} already exists in IDF. Skipping addition."
            )
            self.state["skipped"] += 1
            return
        self.idf.newidfobject(
            "People",
            Name=val_data.name,
            Zone_or_ZoneList_or_Space_or_SpaceList_Name=val_data.zone_or_zonelist_or_space_or_spacelist_name,
            Number_of_People_Schedule_Name=val_data.number_of_people_schedule_name,
            Number_of_People_Calculation_Method=val_data.number_of_people_calculation_method,
            Number_of_People=val_data.number_of_people,
            People_per_Floor_Area=val_data.people_per_floor_area,
            Floor_Area_per_Person=val_data.floor_area_per_person,
            Fraction_Radiant=val_data.fraction_radiant,
            Sensible_Heat_Fraction=val_data.sensible_heat_fraction,
            Activity_Level_Schedule_Name=val_data.activity_level_schedule_name,
            Carbon_Dioxide_Generation_Rate=val_data.carbon_dioxide_generation_rate,
            Enable_ASHRAE_55_Comfort_Warnings=val_data.enable_ashrae_55_comfort_warnings,
            Mean_Radiant_Temperature_Calculation_Type=val_data.mean_radiant_temperature_calculation_type,
            Surface_NameAngle_Factor_List_Name=val_data.surface_name_angle_factor_list_name,
            Work_Efficiency_Schedule_Name=val_data.work_efficiency_schedule_name,
            Clothing_Insulation_Calculation_Method=val_data.clothing_insulation_calculation_method,
            Clothing_Insulation_Calculation_Method_Schedule_Name=val_data.clothing_insulation_calculation_method_schedule_name,
            Clothing_Insulation_Schedule_Name=val_data.clothing_insulation_schedule_name,
            Air_Velocity_Schedule_Name=val_data.air_velocity_schedule_name,
            Thermal_Comfort_Model_1_Type=val_data.thermal_comfort_model_1_type,
            Thermal_Comfort_Model_2_Type=val_data.thermal_comfort_model_2_type,
            Thermal_Comfort_Model_3_Type=val_data.thermal_comfort_model_3_type,
            Thermal_Comfort_Model_4_Type=val_data.thermal_comfort_model_4_type,
            Thermal_Comfort_Model_5_Type=val_data.thermal_comfort_model_5_type,
            Thermal_Comfort_Model_6_Type=val_data.thermal_comfort_model_6_type,
            Thermal_Comfort_Model_7_Type=val_data.thermal_comfort_model_7_type,
            Ankle_Level_Air_Velocity_Schedule_Name=val_data.ankle_level_air_velocity_schedule_name,
            Cold_Stress_Temperature_Threshold=val_data.cold_stress_temperature_threshold,
            Heat_Stress_Temperature_Threshold=val_data.heat_stress_temperature_threshold,
        )
        self.state["success"] += 1
        self.logger.success(f"People with name {val_data.name} added to IDF.")

    def validate(self, data: dict) -> PeopleSchema:
        return PeopleSchema.model_validate(data)
