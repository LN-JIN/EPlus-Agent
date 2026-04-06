from abc import ABC, abstractmethod
from typing import Any, TypedDict

from eppy.modeleditor import IDF

from src.utils.logging import get_logger


class ConvertState(TypedDict):
    success: int
    skipped: int
    failed: int


class BaseConverter(ABC):
    def __init__(self, idf: IDF):
        self.idf = idf
        self.logger = get_logger(__name__)
        self.state: ConvertState = {"success": 0, "skipped": 0, "failed": 0}

    @abstractmethod
    def convert(self, data: dict) -> None: ...

    @abstractmethod
    def _add_to_idf(self, val_data: Any) -> None: ...

    @abstractmethod
    def validate(self, data: dict) -> Any: ...
