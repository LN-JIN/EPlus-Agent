from collections import defaultdict

from eppy.modeleditor import IDF

from src.converters.base_converter import BaseConverter
from src.validator.data_model import GeometrySchema, SurfaceSchema


class SurfaceConverter(BaseConverter):
    def __init__(self, idf: IDF):
        super().__init__(idf)

    def convert(self, data: dict) -> None:
        self.logger.info("Converting BuildingSurface data...")
        surface_data = data.get("BuildingSurface:Detailed", [])
        zone_to_surfaces = defaultdict(list)
        for surface in surface_data:
            zone_to_surfaces[surface["Zone Name"]].append(surface)
        val_data = self.validate(zone_to_surfaces)
        for surface in val_data:
            try:
                self._add_to_idf(surface)
                self.logger.success(
                    f"Successfully converted BuildingSurface: {surface.name}"
                )
                self.state["success"] += 1
            except Exception as e:
                self.state["failed"] += 1
                self.logger.error(
                    f"Error Converting BuildingSurface Data: {e}", exc_info=True
                )

    def _add_to_idf(self, val_data: SurfaceSchema) -> None:
        if self.idf.getobject("BuildingSurface:Detailed", name=val_data.name):
            self.logger.warning(
                f"BuildingSurface with name {val_data.name} already exists in IDF. Skipping addition."
            )
            self.state["skipped"] += 1
            return
        surface_obj = self.idf.newidfobject(
            "BuildingSurface:Detailed",
            Name=val_data.name,
            Surface_Type=val_data.surface_type,
            Construction_Name=val_data.construction_name,
            Zone_Name=val_data.zone_name,
            Space_Name=val_data.space_name or "",
            Outside_Boundary_Condition=val_data.outside_boundary_condition,
            Outside_Boundary_Condition_Object=val_data.outside_boundary_condition_object
            or "",
            Sun_Exposure=val_data.sun_exposure,
            Wind_Exposure=val_data.wind_exposure,
            View_Factor_to_Ground=val_data.view_factor_to_ground,
        )

        for i, vertex in enumerate(val_data.vertices, 1):
            setattr(surface_obj, f"Vertex_{i}_Xcoordinate", vertex[0])
            setattr(surface_obj, f"Vertex_{i}_Ycoordinate", vertex[1])
            setattr(surface_obj, f"Vertex_{i}_Zcoordinate", vertex[2])

    def validate(self, data: dict) -> list[SurfaceSchema]:
        val_data = []
        for _, surfaces in data.items():
            geometry = GeometrySchema.model_validate({"surfaces": surfaces})
            val_data.extend(geometry.surfaces)
        return val_data
