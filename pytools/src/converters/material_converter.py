from typing import Any

from eppy.modeleditor import IDF

from src.converters.base_converter import BaseConverter
from src.utils.logging import get_logger
from src.validator.data_model import (
    AirGapMaterialSchema,
    GlazingMaterialSchema,
    MaterialSchema,
    NoMassMaterialSchema,
    StandardMaterialSchema,
)


class MaterialConverter(BaseConverter):
    """
    Converts material definitions from YAML data into appropriate IDF objects.
    """

    def __init__(self, idf: IDF):
        super().__init__(idf)
        self.logger = get_logger(__name__)

    def convert(self, data: dict[str, Any]) -> None:
        """
        Processes a list of material definitions from the YAML data.
        """
        self.logger.info("Converting Material data...")
        material_list = data.get("Material", [])

        if not material_list:
            self.logger.info("No materials found in YAML data.")
            return

        for material_data in material_list:
            try:
                material_name = material_data.get("Name", "Unknown Material")
                self.logger.debug(f"Processing material: {material_name}")

                validated_material = self.validate(material_data)
                self._add_to_idf(validated_material)

            except Exception as e:
                self.state["failed"] += 1
                self.logger.exception(
                    f"Failed to convert Material '{material_name}': {e!s}"
                )
                continue

    def _add_to_idf(
        self,
        val_data: MaterialSchema
        | StandardMaterialSchema
        | NoMassMaterialSchema
        | AirGapMaterialSchema
        | GlazingMaterialSchema,
    ) -> None:
        try:
            idf_key = self._get_idf_key(val_data.type)

            if not idf_key:
                self.logger.error(
                    f"Unknown material type '{val_data.type}' for material '{val_data.name}'"
                )
                self.state["failed"] += 1
                return

            if self.idf.getobject(idf_key, val_data.name):
                self.logger.warning(
                    f"{idf_key} with name '{val_data.name}' already exists. Skipping addition."
                )
                self.state["skipped"] += 1
                return
            if isinstance(val_data, StandardMaterialSchema):
                self._add_standard_material_to_idf(val_data)
            elif isinstance(val_data, NoMassMaterialSchema):
                self._add_no_mass_material_to_idf(val_data)
            elif isinstance(val_data, AirGapMaterialSchema):
                self._add_air_gap_material_to_idf(val_data)
            elif isinstance(val_data, GlazingMaterialSchema):
                self._add_glazing_material_to_idf(val_data)
            self.state["success"] += 1
            self.logger.success(f"Material '{val_data.name}' added successfully.")
        except Exception:
            self.state["failed"] += 1
            self.logger.exception(
                f"An unexpected error occurred while adding material '{val_data.name}' to IDF"
            )

    def _get_idf_key(self, material_type: str) -> str:
        type_to_key: dict[str, str] = {
            "Standard": "Material",
            "NoMass": "Material:NoMass",
            "AirGap": "Material:AirGap",
            "Glazing": "WindowMaterial:SimpleGlazingSystem",
        }
        return type_to_key.get(material_type) or ""

    def validate(self, data: dict) -> MaterialSchema:
        """
        Validates material data using the MaterialSchema.
        """
        return MaterialSchema.model_validate(data)

    def _add_standard_material_to_idf(self, material: StandardMaterialSchema) -> None:
        self.idf.newidfobject(
            "Material",
            Name=material.name,
            Roughness=material.roughness,
            Thickness=material.thickness,
            Conductivity=material.conductivity,
            Density=material.density,
            Specific_Heat=material.specific_heat,
        )

    def _add_no_mass_material_to_idf(self, material: NoMassMaterialSchema) -> None:
        self.idf.newidfobject(
            "Material:NoMass",
            Name=material.name,
            Roughness=material.roughness,
            Thermal_Resistance=material.thermal_resistance,
        )

    def _add_air_gap_material_to_idf(self, material: AirGapMaterialSchema) -> None:
        self.idf.newidfobject(
            "Material:AirGap",
            Name=material.name,
            Thermal_Resistance=material.thermal_resistance,
        )

    def _add_glazing_material_to_idf(self, material: GlazingMaterialSchema) -> None:
        self.idf.newidfobject(
            "WindowMaterial:SimpleGlazingSystem",
            Name=material.name,
            UFactor=material.u_factor,
            Solar_Heat_Gain_Coefficient=material.solar_heat_gain_coefficient,
            Visible_Transmittance=material.visible_transmittance or "",
        )
