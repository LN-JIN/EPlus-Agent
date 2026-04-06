import time
from pathlib import Path
from typing import Optional

import typer

from src.converter_manager import ConverterManager
from src.runner.runner import EnergyPlusRunner
from src.utils.logging import get_logger, setup_logger
from src.validator.data_model import BaseSchema

logger_time = time.strftime("%Y%m%d_%H%M%S")
setup_logger(
    level="INFO",
    console_output=True,
    log_file_path=Path(f"./output/logs/{logger_time}.log"),
)
logger = get_logger(__name__)

app = typer.Typer()

idd_file = Path("./data/dependencies/Energy+.idd")
BaseSchema.set_idf(idd_file)


@app.command()
def convert_idf(
    yaml_file: Path = typer.Argument(..., help="YAML 配置文件路径"),
    output_dir: Optional[Path] = typer.Option(None, help="IDF 输出目录（默认 ./output/idf/）"),
):
    """将 YAML 配置文件转换为 EnergyPlus IDF 文件（不执行仿真）。
    成功时打印 IDF_OUTPUT: <绝对路径>，exit 0；失败 exit 1。
    """
    try:
        yaml_stem = yaml_file.stem
        idf_out = (output_dir or Path("./output/idf")) / f"{yaml_stem}.idf"
        idf_out.parent.mkdir(parents=True, exist_ok=True)
        manager = ConverterManager(yaml_file)
        manager.convert_all()
        manager.save_idf(idf_out)
        print(f"IDF_OUTPUT: {idf_out.resolve()}")
        logger.info(f"IDF 生成成功: {idf_out.resolve()}")
    except Exception as e:
        logger.error(f"IDF 转换失败: {e}")
        print(f"IDF_ERROR: {e}")
        raise typer.Exit(code=1)


@app.command()
def run_simulation(
    idf_file: Path = typer.Argument(..., help="IDF 文件路径"),
    epw_file: Path = typer.Option(Path("./data/weather/Shenzhen.epw"), help="EPW 气象文件路径"),
    output_dir: Optional[Path] = typer.Option(None, help="仿真输出目录（默认按 IDF 文件名生成）"),
):
    """执行 EnergyPlus 仿真（不做 YAML 转换）。
    成功时打印 SIMULATION_OUTPUT_DIR: <绝对路径>，exit 0；失败 exit 1。
    """
    try:
        stem = idf_file.stem
        sim_out = output_dir or Path(f"./output/results/{stem}")
        runner = EnergyPlusRunner(idd_file_path=idd_file)
        success = runner.run_idf(
            epw_file_path=epw_file,
            idf_file_path=idf_file,
            output_directory=sim_out,
        )
        print(f"SIMULATION_OUTPUT_DIR: {sim_out.resolve()}")
        if not success:
            logger.error("EnergyPlus 仿真失败（非零退出码）")
            raise typer.Exit(code=1)
        logger.info(f"仿真成功完成，输出目录: {sim_out.resolve()}")
    except typer.Exit:
        raise
    except Exception as e:
        logger.error(f"仿真执行异常: {e}")
        print(f"SIMULATION_ERROR: {e}")
        raise typer.Exit(code=1)


@app.command()
def validate_idf(
    idf_file: Path = typer.Argument(..., help="IDF 文件路径"),
):
    """验证 IDF 文件语法合法性（不执行仿真）。
    合法时打印 IDF_VALID: True，exit 0；非法时打印 IDF_VALID: False，exit 1。
    """
    try:
        from eppy.modeleditor import IDF
        IDF.setiddname(str(idd_file))
        idf = IDF(str(idf_file))
        zones = idf.idfobjects.get("ZONE", [])
        print(f"IDF_VALID: True, zones={len(zones)}")
        logger.info(f"IDF 验证通过，区域数: {len(zones)}")
    except Exception as e:
        print(f"IDF_VALID: False, error={e}")
        logger.error(f"IDF 验证失败: {e}")
        raise typer.Exit(code=1)


@app.command()
def edit_idf(
    idf_file: Path = typer.Argument(..., help="IDF 文件路径"),
    object_type: str = typer.Option(..., help="EnergyPlus 对象类型，如 ZoneHVAC:IdealLoadsAirSystem"),
    name: str = typer.Option(..., help="对象的 Name 字段值"),
    field: str = typer.Option(..., help="要修改的字段名（eppy 属性名）"),
    value: str = typer.Option(..., help="新字段值（字符串，eppy 会自动转型）"),
):
    """修改 IDF 文件中指定对象的字段值（使用 eppy）。
    成功时打印 EDIT_OK: <object_type>/<name>.<field> = <value>，exit 0；失败 exit 1。
    """
    try:
        from eppy.modeleditor import IDF
        IDF.setiddname(str(idd_file))
        idf = IDF(str(idf_file))

        # 查找指定对象（大写匹配）
        objs = [
            o for o in idf.idfobjects.get(object_type.upper(), [])
            if getattr(o, "Name", "") == name
        ]
        if not objs:
            print(f"EDIT_ERROR: object not found: {object_type}/{name}")
            logger.error(f"未找到对象: {object_type}/{name}")
            raise typer.Exit(code=1)

        setattr(objs[0], field, value)
        idf.save()
        print(f"EDIT_OK: {object_type}/{name}.{field} = {value}")
        logger.info(f"IDF 修改成功: {object_type}/{name}.{field} = {value}")
    except typer.Exit:
        raise
    except AttributeError as e:
        print(f"EDIT_ERROR: invalid field '{field}': {e}")
        logger.error(f"无效字段名 '{field}': {e}")
        raise typer.Exit(code=1)
    except Exception as e:
        print(f"EDIT_ERROR: {e}")
        logger.error(f"IDF 修改异常: {e}")
        raise typer.Exit(code=1)


@app.command()
def read_idf(
    idf_file: Path = typer.Argument(..., help="IDF 文件路径"),
    object_type: str = typer.Option(..., "--object-type", help="对象类型，如 Material"),
):
    """读取 IDF 文件中指定类型对象的名称和字段值。
    成功时打印 READ_IDF_OBJECTS: <JSON>，exit 0；失败 exit 1。
    """
    try:
        import json
        from eppy.modeleditor import IDF
        IDF.setiddname(str(idd_file))
        idf = IDF(str(idf_file))
        objs = idf.idfobjects.get(object_type.upper(), [])
        result = []
        for obj in objs:
            fields = {}
            for fname in obj.fieldnames:
                try:
                    fields[fname] = str(getattr(obj, fname, ""))
                except Exception:
                    pass
            result.append({"name": getattr(obj, "Name", ""), "fields": fields})
        print(f"READ_IDF_OBJECTS: {json.dumps(result, ensure_ascii=False)}")
        logger.info(f"IDF 对象读取成功: {object_type}, 共 {len(result)} 个")
    except Exception as e:
        print(f"READ_IDF_OBJECTS: []")
        logger.error(f"IDF 对象读取失败: {e}")
        raise typer.Exit(code=1)


if __name__ == "__main__":
    app()
