package dataset

import (
    "fmt"
    "os"
    "time"
)

type multiSet map[string]DataSet

type DataSetLog struct {
    Data        multiSet
    ProcessName string
    Path        string
    FileTag     string
    Header      string
};

type DataValue struct {
    label     string // Description of value.
    svalue    string // Alternate value for tags
    value     any    // holds the data, but only suports POD types and byte slices
    precision int    // Precision or width of value in output ascii file.
    first     bool   // indicates if this is the first column in the data set
}

type DataSet struct {
    array   []DataValue;
    size    uint
    maxsize uint
    Header  string
}

func (d *DataSetLog) GetDataSet(id string) *DataSet {
    v, found := d.Data[id]
    if !found {
        return nil
    }

    if v.size < v.maxsize {
        return &v;
    }

    return nil
}
func (d *DataSetLog) DumpDataSetToDiskFile() error {
    fileEnd := d.FileTag + ".set"
    if d.FileTag == "" {
        now := time.Now().UTC()
        ymd := fmt.Sprintf("%d%02d%02d", now.Year(), now.Month(), now.Day())
        hm := fmt.Sprintf("%02d%02d", now.Hour(), now.Minute())
        fileEnd = "_" + ymd + "_" + hm + ".set";
    }

    var fileName string

    var val DataValue
    for dkey, dset := range d.Data {

        //Don't try to write file if no entries saved.
        if dset.size > 0 {
            //Set up file and output parameters.
            fileName = d.Path + d.ProcessName + "_" + dkey + fileEnd

            file, err := os.OpenFile(fileName, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0700)
            if err != nil {
                //open failed, try to dump to local directory.
                //clear with no parameter => 0 => clears all bits of ios error state.
                fileName = d.ProcessName + "_" + dkey + fileEnd;
                file, err = os.OpenFile(fileName, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0700)
                if err != nil {
                    return err
                }
            }

            if d.Header != "" {
                file.WriteString(d.Header)
            }

            if dset.Header != "" {
                file.WriteString(dset.Header)
            }

            inHeader := true
            spacer := "# "
            for ival := uint(1); inHeader && ival < dset.size; ival++ {
                val = dset.array[ival]
                width := val.precision
                switch val.value.(type) {
                case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
                    width = val.precision
                case float64, float32:
                    width += 7
                case string:
                    width = val.precision
                case []byte:
                    width = val.precision
                default:
                    width = 0
                }
                label := val.label[:width]
                file.WriteString(spacer)
                fmtString := fmt.Sprintf("%%%ds", width)
                file.WriteString(fmt.Sprintf(fmtString, label))
                spacer = "  ";
                if dset.array[ival].first {
                    inHeader = false
                }
            }
            file.WriteString("\n")

            //loop through data values
            for ival := uint(0); ival < dset.size; ival++ {
                val = dset.array[ival];
                if val.first {
                    file.WriteString("\n")
                }
                var out string
                width := val.precision
                switch v := val.value.(type) {
                case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
                    fmtString := fmt.Sprintf("%%%dd", val.precision)
                    out = fmt.Sprintf(fmtString, v)
                case float64, float32:
                    width += 7
                    fmtString := fmt.Sprintf("%%%d.%df", val.precision+7, val.precision)
                    out = fmt.Sprintf(fmtString, v)
                case string:
                    width = val.precision
                    fmtString := fmt.Sprintf("%%%ds", width)
                    out = fmt.Sprintf(fmtString, val.value)
                case []byte:
                    width = val.precision
                    out = fmt.Sprintf("%x", val.value)
                default:
                    out = fmt.Sprintf("unknown_type:%v", v)
                }
                file.WriteString(out)
            }
            file.WriteString("\n")

            if dset.size >= dset.maxsize {
                file.WriteString("\n#Dataset may have been closed due to max memory limit.\n")
            }
            //Close out file
            file.Close()

            //Reset array size to zero so it can be reused
            //Do not de-allocate the storage
            dset.size = 0;
        }

    }

    return nil
}

type Options struct {
    Header      string
    MaxDataSize uint
}

func DefaultOptions() Options {
    return Options{"", 200000}
}

func (d *DataSetLog) Initialize(key string, opts Options) {
    if d.Data == nil {
        d.Data = make(map[string]DataSet)
    }

    d.Data[key] = DataSet{array: make([]DataValue, opts.MaxDataSize), maxsize: opts.MaxDataSize, Header: opts.Header}
}

func (d *DataSetLog) SetFileTag(part1 string, part2 string) {
    d.FileTag = "_" + part1;
    if part2 != "" {
        d.FileTag += "_" + part2;
    }
}

func (d *DataSet) Save(label string, value any, precisionOrWidth int, first bool) *DataSet {
    if d.size < d.maxsize {
        dv := &d.array[d.size]
        d.size++

        dv.label = label
        dv.value = value;
        dv.precision = precisionOrWidth
        dv.first = first;
    }

    return nil
}

