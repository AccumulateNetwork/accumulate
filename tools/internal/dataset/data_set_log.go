package dataset

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type multiSet map[string]*DataSet

type DataSetLog struct {
	data        multiSet
	processName string
	path        string
	fileTag     string
	header      string
	mutex       sync.Mutex
}

type DataValue struct {
	label     string // Description of value.
	svalue    string // Alternate value for tags
	value     any    // holds the data, but only suports POD types and byte slices
	precision int    // Precision or width of value in output ascii file.
	first     bool   // indicates if this is the first column in the data set
}

type DataSet struct {
	array   []DataValue
	size    uint
	maxsize uint
	header  string
	mutex   sync.Mutex
}

func (d *DataSetLog) SetProcessName(process string) {
	d.processName = process
}

func (d *DataSetLog) SetPath(path string) {
	d.path = path
}

func (d *DataSetLog) SetHeader(header string) {
	d.header = header
}

func (d *DataSetLog) GetDataSet(id string) *DataSet {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	v, found := d.data[id]
	if !found {
		return nil
	}

	if v.size < v.maxsize {
		return v
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a

	}
	return b
}

func write(file *os.File, out string) error {
	_, err := file.WriteString(out)
	if err != nil {
		file.Close()
		return err
	}
	return nil
}

func (d *DataSetLog) DumpDataSetToDiskFile() ([]string, error) {
	fileEnd := d.fileTag + ".dat"
	if d.fileTag == "" {
		now := time.Now().UTC()
		ymd := fmt.Sprintf("%d%02d%02d", now.Year(), now.Month(), now.Day())
		hm := fmt.Sprintf("%02d%02d", now.Hour(), now.Minute())
		fileEnd = "_" + ymd + "_" + hm + ".dat"
	}

	var fileNames []string

	var fileName string

	var val DataValue
	for dkey, dset := range d.data {
		//Don't try to write file if no entries saved.
		if dset.size > 0 {
			//Set up file and output parameters.
			fileName = d.path + d.processName + "_" + dkey + fileEnd
			fileNames = append(fileNames, fileName)

			file, err := os.OpenFile(fileName, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0700)
			if err != nil {
				//open failed, try to dump to local directory.
				//clear with no parameter => 0 => clears all bits of ios error state.
				fileName = d.processName + "_" + dkey + fileEnd
				file, err = os.OpenFile(fileName, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0700)
				if err != nil {
					return nil, err
				}
			}

			if d.header != "" {
				err = write(file, d.header)
				return nil, err
			}

			if dset.header != "" {
				err = write(file, dset.header)
				return nil, err
			}

			spacer := "# "
			for ival := uint(0); ival < dset.size; ival++ {
				if dset.array[ival].first && ival != 0 {
					break
				}
				val = dset.array[ival]
				width := val.precision
				switch val.value.(type) {
				case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, []byte, string:
					width = val.precision
				case float64, float32:
					width += 7
				default:
					width = len(val.label)
				}
				label := val.label[:min(len(val.label), width)]
				fmtString := fmt.Sprintf("%s%%-%ds", spacer, width)
				err = write(file, fmt.Sprintf(fmtString, label))
				if err != nil {
					return nil, err
				}
				spacer = "  "
			}
			//loop through data values
			for ival := uint(0); ival < dset.size; ival++ {
				val = dset.array[ival]
				var out string
				if val.first {
					out = "\n  "
				}
				switch v := val.value.(type) {
				case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
					fmtString := fmt.Sprintf("%%-%dd", val.precision)
					out += fmt.Sprintf(fmtString, v)
				case float64, float32:
					fmtString := fmt.Sprintf("%%-%d.%df", val.precision+7, val.precision)
					out += fmt.Sprintf(fmtString, v)
				case string:
					fmtString := fmt.Sprintf("%%-%ds", val.precision)
					out += fmt.Sprintf(fmtString, val.value)
				case []byte:
					fmtString := fmt.Sprintf("%%-%dx", val.precision)
					out += fmt.Sprintf(fmtString, val.value)
				default:
					out += fmt.Sprintf("unknown_type:%v", v)
				}
				out += spacer
				err = write(file, out)
				if err != nil {
					return nil, err
				}
			}
			err = write(file, "\n")
			if err != nil {
				return nil, err
			}

			if dset.size >= dset.maxsize {
				err = write(file, "\n#Dataset may have been closed due to max memory limit.\n")
				return nil, err
			}

			//Reset array size to zero so it can be reused
			//Do not de-allocate the storage
			dset.size = 0

			// all done
			file.Close()
		}

	}

	return fileNames, nil
}

type Options struct {
	Header      string
	MaxDataSize uint
}

func DefaultOptions() Options {
	return Options{"", 200000}
}

func (d *DataSetLog) Initialize(key string, opts Options) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.data == nil {
		d.data = make(map[string]*DataSet)
	}

	d.data[key] = &DataSet{array: make([]DataValue, opts.MaxDataSize), maxsize: opts.MaxDataSize, header: opts.Header}
}

func (d *DataSetLog) SetFileTag(part1 string, part2 string) {
	d.fileTag = "_" + part1
	if part2 != "" {
		d.fileTag += "_" + part2
	}
}

func (d *DataSet) Save(label string, value any, precisionOrWidth int, first bool) *DataSet {
	if d.size < d.maxsize {
		dv := &d.array[d.size]
		d.size++

		dv.label = label
		dv.value = value
		dv.precision = precisionOrWidth
		dv.first = first
	}

	return d
}

func (d *DataSet) Lock() *DataSet {
	d.mutex.Lock()
	return d
}

func (d *DataSet) Unlock() {
	d.mutex.Unlock()
}

func (d *DataSet) SetHeader(header string) {
	d.header = header
}
