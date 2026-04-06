from copy import deepcopy
from io import StringIO
from pathlib import Path
from typing import cast

import yaml
from eppy.modeleditor import IDF

from src.converters import (
    BuildingConverter,
    ConstructionConverter,
    FenestrationConverter,
    HVACConverter,
    LightConverter,
    MaterialConverter,
    PeopleConverter,
    ScheduleConverter,
    SettingsConverter,
    SurfaceConverter,
    ZoneConverter,
)
from src.utils.logging import get_logger
from src.validator.data_model import BaseSchema, IDDField


class ConverterManager:
    def __init__(self, file_to_convert: Path):
        self.logger = get_logger(__name__)
        self._idf = BaseSchema.get_idf()
        self.yaml_data: dict = self._load_yaml(file_to_convert)
        self.converters = {
            "settings": SettingsConverter(self._idf),
            "building": BuildingConverter(self._idf),
            "schedules": ScheduleConverter(self._idf),
            "zones": ZoneConverter(self._idf),
            "materials": MaterialConverter(self._idf),
            "constructions": ConstructionConverter(self._idf),
            "surfaces": SurfaceConverter(self._idf),
            "fenestrations": FenestrationConverter(self._idf),
            "hvac": HVACConverter(self._idf),
            "lights": LightConverter(self._idf),
            "people": PeopleConverter(self._idf),
        }

    @property
    def idf(self) -> IDF:
        return deepcopy(self._idf)

    def convert_all(self) -> None:
        for name, converter in self.converters.items():
            self.logger.info(f"Converting {name}...")
            converter.convert(self.yaml_data)

    def save_idf(self, output_path: Path) -> None:
        output_path.parent.mkdir(parents=True, exist_ok=True)
        self.logger.info(f"Saving IDF to {output_path}...")
        self._idf.saveas(str(output_path))

    def load_idf(self, idf_path: Path) -> None:
        self.logger.info(f"Loading IDF from {idf_path}...")
        self._idf = IDF(str(idf_path))
        for converter in self.converters.values():
            converter.idf = self._idf

    def _create_blank_idf(self) -> IDF:
        self.logger.info("Creating a blank IDF instance.")
        idf_text = ""
        fhandle = StringIO(idf_text)
        return IDF(fhandle)

    def _load_yaml(self, file_path: Path) -> dict:
        self.logger.info(f"Loading YAML file from {file_path}.")
        with open(file_path, encoding="utf-8") as f:
            return yaml.safe_load(f)

    def _process_idf_field(self) -> IDDField:
        _idd_info = cast(list[dict], self._idf.idd_info)
        idd_field = IDDField(_idd_info)
        return idd_field
