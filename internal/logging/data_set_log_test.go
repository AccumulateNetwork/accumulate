package dataset

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataSetLog(t *testing.T) {
	dsl := DataSetLog{}
	path, err := ioutil.TempDir("", "DataSetTest")
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(path, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(path)
	dsl.SetPath(path)
	dsl.SetProcessName("data_set_test")

	dsl.Initialize("test1", DefaultOptions())
	dsl.Initialize("test2", DefaultOptions())

	ds := dsl.GetDataSet("test1")
	require.True(t, ds != nil, "data set buffer has not been initialized")

	time := float64(0.0)
	delta := float32(0.5)
	for i := 0; i < 10; i++ {
		url := "acc://test1adi" + strconv.Itoa(i+1)
		h := sha256.Sum256([]byte(url))
		time += float64(delta)
		t2 := float32(i) * delta
		ds.Lock().Save("int_test", i, 10, true).
			Save("float64_test", time, 10, false).
			Save("float32_test", t2, 10, false).
			Save("string_test", url, 32, false).
			Save("slice_test", h[:], len(h), false).Unlock()
	}

	ds = dsl.GetDataSet("test2")
	require.True(t, ds != nil, "data set buffer has not been initialized")

	time = float64(0.0)
	delta = float32(0.1)
	for i := 0; i < 100; i++ {
		url := "acc://test2adi" + strconv.Itoa(i+1)
		h := sha256.Sum256([]byte(url))
		time += float64(delta)
		t2 := float32(i) * delta * delta
		ds.Lock().Save("int_test2", i, 10, true).
			Save("float64_test2", time, 10, false).
			Save("float32_test2", t2, 10, false).
			Save("string_test2", url, 32, false).
			Save("slice_test2", h[:], len(h), false).Unlock()
	}

	files, err := dsl.DumpDataSetToDiskFile()
	require.NoError(t, err, "error dumping data set log")

	for _, file := range files {
		f, err := os.Open(file)
		require.NoError(t, err)
		defer f.Close()
		scanner := bufio.NewScanner(f)
		t.Logf("%s\n", file)

		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}

		fmt.Println()
		fmt.Println()
		if err := scanner.Err(); err != nil {
			require.NoError(t, err)
		}
	}
}
