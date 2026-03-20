# Materials Science EM Metadata Extractor

A Python script that extracts metadata from Electron Microscopy (EM) data for Materials Science (MS).
The extractor is wrapped by a GoLang orchestrator, which will feed the extracted metadata to the [Converter](https://github.com/osc-em/oscem-converter-extracted), for conversion to [OSC-EM schema](https://github.com/osc-em/oscem-schemas).
Currently supported file formats: `.emd`, `.prz`.

## Description

All materials science data files to be studied so far within the scope of OpenEM are in some form of hdf5 format.
To read them and extract their metadata, we are using extrernal libraries suited for this task.
These extrernal dependecies are the reason this extractor is written in Python, as Go does not support all of them.
In the rare case that a file cannot be processed by a library, the fallback solution is to use numpy, since all hdf5 files can also be read as numpy files.


## Extractor Code

Located in `extractor/__main__.py`.

Working for both `.emd` and `.prz` files, with the exception of some types of `.prz` files, which are still unsupported.
For these unsupported files we use `numpy` for manual extraction.
Note that the metadata of those will not be as complete.

### Dependencies
Python 3.12.3, plus those found in [requirements.txt](./requirements.txt)

### Usage
You can directly run the extractor as it is, with `python3 -m extractor <input_directory>`.

For each file inside <input_directory> with name <file_name>, the script will print out the metadata results.
If you uncomment the relevant part of the code in the script, then it will also create a metadata file named "<file_name>_metadata.json" for each file.
By default, it will create a folder named "ms_extractor_results" where all the outputs will be written.

## Go Wrapper

To be able to run both Python and Go code together, and to avoid introducing external dependencies, we bundle the extractor into an executable and use a Go wrapper script.
We use `PyInstaller` to create the executable.

### PyInstaller Guide

PyInstaller is a tool that bundles Python applications and all their dependencies into a single executable file.
It packages Python code as bytecode along with a Python interpreter, making it run on systems without Python installed.

#### Step-by-Step Instructions

**1. Set Up Environment:**
Create and activate a Python 3.12.3 virtual environment and install the extractor dependencies and PyInstaller:

```bash
python3.12 -m venv ms_extractor
source ms_extractor/bin/activate
python -m pip install --upgrade pip
pip install -r requirements.txt pyinstaller
```

**2. Usage:**
```bash
pyinstaller --onefile --name=extractor_bin extractor/__main__.py --hidden-import=numpy.core._multiarray_umath --hidden-import=numpy.core._multiarray --collect-all=numpy
```

Where:
- `--onefile`: Creates single executable (slower startup; extracts to temp directory)
- `--name=extractor_bin`: Custom name for the executable
- `--hidden-import`: Add these flags to ensure numpy compatibility
- PyInstaller automatically detected and included:
  - `rsciio` and its dependencies
  - `numpy` and related libraries
  - `h5py` for HDF5 file support

This creates:
- `dist/` directory with your executable
- `build/` directory with temporary files
- `.spec` file (build configuration)

**3. After compilation:**
```bash
# Check file type
file dist/extractor_bin

# Check size
ls -lh dist/extractor_bin

# Test the executable
./dist/extractor_bin <input_directory>
```

**4. Cleanup:**
After a successful build, you can remove the temporary files.

```bash
rm -rf build/
rm *.spec  # if you don't need to customize
```

`extractor_bin` is the only thing you need for local development and, as long as it is in the `/dist` directory, it will be automatically detected by the orchestrator.

### Using the executable

The Python extractor is bundled as an executable and invoked by the Go orchestrator (`main.go`).
This means that, after completing the metadata extraction, the Go orchestrator will then call the module for convertion to OSC-EM schema.

The orchestrator expects **two required flagged arguments**:
- `-i <input_directory>` (required): directory holding the single data file to process
- `-o <output_file>` (required): output path for the converted OSC-EM result

Optional development override:
- `-e <path>` (optional): local path to a different extractor (developer override)

#### Usage:

> Build and run from a cloned repo (assets provided locally):
```
go build -o ms_reader main.go
./ms_reader -i /path/to/input -o <output_file>.json
```

> Run using another extractor (developer override):
```
./ms_reader -i /path/to/input -o <output_file>.json -e /path/to/extractor_bin
```

#### Notes:
For MS we assume that **one experiment (one dataset) will only consist of one file**.
Hence, **<input_directory> should contain one file each time**.
That is, the current version of the MS metadata extractor does not cover cases where a dataset may consist of multiple files.

For the converter to understand which MS conversion table to use, we read the type of the file inside <input_directory> and instruct the Go module to use the revelant table.
Currently there are two such tables: one for `.emd` and one for `.prz` files.

Keep in mind that:

- The code expects a single file inside `<input_directory>` (one dataset per run).
- For the converter to choose the correct table we read the file type inside `<input_directory>`. Currently supported tables are for `.emd` and `.prz` files.
- For the program to run locally we need to provide the required extractor executable.
- No locally built binary should be committed to the repo.
