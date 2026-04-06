from eppy.modeleditor import IDF

from src.converters.base_converter import BaseConverter
from src.validator.data_model import (
    FenestrationSurfaceSchema,
    GeometrySchema,
)


class FenestrationConverter(BaseConverter):
    def __init__(self, idf: IDF):
        super().__init__(idf)

    def convert(self, data: dict) -> None:
        self.logger.info("Converting FenestrationSurface data...")
        fenestration_data = data.get("FenestrationSurface:Detailed", [])

        val_data = self.validate({"fenestrationsurfaces": fenestration_data})
        for fenestration in val_data.fenestrationsurfaces:
            try:
                self._add_to_idf(fenestration)
                self.logger.success(
                    f"Successfully converted FenestrationSurface: {fenestration.name}"
                )
                self.state["success"] += 1
            except Exception as e:
                self.state["failed"] += 1
                self.logger.error(
                    f"Error Converting FenestrationSurface Data: {e}", exc_info=True
                )

    def _add_to_idf(self, val_data: FenestrationSurfaceSchema) -> None:
        if self.idf.getobject("FenestrationSurface:Detailed", name=val_data.name):
            self.logger.warning(
                f"FenestrationSurface with name {val_data.name} already exists in IDF. Skipping addition."
            )
            self.state["skipped"] += 1
            return

        if self.idf.getobject("Construction", name=val_data.construction_name) is None:
            raise ValueError(
                f"Construction {val_data.construction_name} does not exist in IDF"
            )

        fenestration_obj = self.idf.newidfobject(
            "FenestrationSurface:Detailed",
            Name=val_data.name,
            Surface_Type=val_data.surface_type,
            Construction_Name=val_data.construction_name,
            Building_Surface_Name=val_data.building_surface_name,
            Outside_Boundary_Condition_Object=val_data.outside_boundary_condition_object
            or "",
            View_Factor_to_Ground=val_data.view_factor_to_ground or "",
            Frame_and_Divider_Name=val_data.frame_and_divider_name or "",
            Multiplier=val_data.multiplier,
            Number_of_Vertices=val_data.Number_of_Vertices,
        )

        for i, vertex in enumerate(val_data.vertices, 1):
            setattr(fenestration_obj, f"Vertex_{i}_Xcoordinate", vertex[0])
            setattr(fenestration_obj, f"Vertex_{i}_Ycoordinate", vertex[1])
            setattr(fenestration_obj, f"Vertex_{i}_Zcoordinate", vertex[2])

    def validate(self, data: dict) -> GeometrySchema:
        try:
            geometry = GeometrySchema.model_validate(data)
        except Exception as e:
            self.logger.error(
                f"Geometry validation failed for fenestration surfaces: {e}"
            )
            self.state["failed"] += len(data)
        return geometry
