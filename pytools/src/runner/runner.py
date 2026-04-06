import shutil
import subprocess
from datetime import datetime
from io import StringIO
from pathlib import Path
from typing import cast

from eppy.modeleditor import IDF

from src.utils.logging import get_logger


class EnergyPlusRunner:
    def __init__(self, idf: IDF | None = None, idd_file_path: Path | None = None):
        """
        Initialize the EnergyPlusRunner.

        Args:
            idf: An instance of eppy.modeleditor.IDF
            idd_file_path: EnergyPlus IDD file path, required if idf is not provided
        """
        self.logger = get_logger(__name__)
        if idf:
            self.idf = idf
        else:
            try:
                IDF.setiddname(str(idd_file_path))
                self.idf = IDF(StringIO(""))
            except Exception as e:
                self.logger.error(
                    f"Must provide either an IDF instance or a valid IDD file path. Error: {e}"
                )
                raise

        self.logger.info("EnergyPlusRunner initialized.")

    def run_idf(
        self,
        epw_file_path: Path | str,
        idf_file_path: Path | str | None = None,
        output_directory: Path | None = None,
    ) -> bool:
        """
        Run EnergyPlus IDF file

        Args:
            idf_file_path: IDF file path
            epw_file_path: EPW weather file path
            output_directory: Output directory, if None, a default directory will be created

        Returns:
            bool: True if the simulation ran successfully, False otherwise
        """
        if idf_file_path:
            self.idf_path = Path(idf_file_path)
            self.idf = IDF(str(self.idf_path))
        elif self.idf.idfname:
            idf_file_path = cast(str, self.idf.idfname)
            self.idf_path = Path(idf_file_path)
        else:
            raise ValueError(
                "IDF file path must be provided either via parameter or IDF instance."
            )
        self.epw_path = Path(epw_file_path)

        if not self.idf_path.exists():
            raise FileNotFoundError(f"IDF file not found: {self.idf_path}")
        if not self.epw_path.exists():
            raise FileNotFoundError(f"EPW file not found: {self.epw_path}")

        if output_directory is None:
            output_directory = (
                Path(__file__).parent.parent.parent
                / "output"
                / "results"
                / f"energyplus_runs_{datetime.now().strftime('%Y%m%d_%H%M%S')}"
            )
        else:
            output_directory = Path(output_directory)
        output_directory.mkdir(parents=True, exist_ok=True)

        self.logger.info("Starting EnergyPlus simulation...")
        self.logger.info(f"IDF file: {self.idf_path}")
        self.logger.info(f"EPW file: {self.epw_path}")
        self.logger.info(f"Output directory: {output_directory}")

        try:
            energyplus_exe = shutil.which("energyplus")
            if not energyplus_exe:
                raise FileNotFoundError("EnergyPlus executable not found in PATH")

            cmd = [
                energyplus_exe,
                "-w",
                str(self.epw_path),
                "-d",
                str(output_directory),
                "-r",
                str(self.idf_path),
            ]

            self.logger.info(f"Running command: {' '.join(cmd)}")

            process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                bufsize=1,
            )

            output_lines = []
            for line in process.stdout or []:
                line = line.rstrip()
                self.logger.info(f"[EnergyPlus] {line}")
                output_lines.append(line)

            return_code = process.wait()

            if return_code != 0:
                self.logger.error(f"EnergyPlus exited with code {return_code}")
                return False

            self.logger.info("EnergyPlus simulation completed successfully.")
            return True

        except FileNotFoundError:
            self.logger.error("EnergyPlus executable not found.")
            return False

        except Exception as e:
            self.logger.exception(f"Running EnergyPlus simulation failed: {e}")
            raise
