# Range variable reference capture

The `rangevarref` static analyzer will detect the issue described here. The
analyzer has many false positives because determining if a variable escapes the
stack is hard and a false negative could cause a serious bug. Better safe than
sorry.

```go
func forLoop() {
    source := []int{1, 2, 3}
    var values []*int
    var x int
    for i := 0; i < len(source); i++ {
        x = source[i]
        values = append(values, &x)
    }
    fmt.Println(values) // -> [3 3 3]
}

func takeAddress() {
    var values []*int
    for _, x := range []int{1, 2, 3} {
        values = append(values, &x)
    }
    fmt.Println(values) // -> [3 3 3]
}

func sliceArray() {
    var values [][]int
    for _, x := range [][1]int{{1}, {2}, {3}} {
        values = append(values, x[:])
    }
    fmt.Println(values) // -> [[3] [3] [3]]
}
```

The functions above have a problem: they capture a reference to a value-type
range variable. It is more obvious in `forLoop`, but `takeAddress` is
functionaly identical to `forLoop`, and `sliceArray` has the same fundamental
problem. The issue is:

- The Go compiler allocates a variable for the range value
- This variable is overwritten by each loop iteration
- The loop body captures a pointer to the variable

Thus, each iteration of the loop captures a pointer to the same variable, which
is overwritten each iteration. The end result is that `values` holds three
copies of the same pointer.

An easy way around this problem is to redeclare the range variable:

```go
for _, x := range []int{1, 2, 3} {
    x := x // See docs/developer/rangevarref.md
    values = append(values, &x)
}
```

This creates a new variable that is limited to the scope of the loop body. If
the new variable escapes the stack, Go will allocate a new heap variable for
each loop iteration, preventing the issue.

## Comment from smt/storage/badger/batch.go

> The statement below takes a copy of K. This is necessary because K is `var k
> [32]byte`, a fixed-length array, and arrays in go are pass-by-value. This
> means that range variable K is overwritten on each loop iteration. Without
> this statement, `k[:]` creates a slice that points to the range variable, so
> every call to `txn.Set` gets a slice pointing to the same memory. Since the
> transaction defers the actual write until `txn.Commit` is called, it saves the
> slice. And since all of the slices are pointing to the same variable, and that
> variable is overwritten on each iteration, the slices held by `txn` all point
> to the same value. When the transaction is committed, every value is written
> to the last key. Taking a copy solves this because each loop iteration creates
> a new copy, and `k[:]` references that copy instead of the original. See also:
> https://github.com/golang/go/wiki/CommonMistakes#using-reference-to-loop-iterator-variable

This comment was referring to the following code:

```go
for k, v := range values {
    // The comment was here
    k := k
    err := b.txn.Set(k[:], v)
    if err != nil {
        return err
    }
}
```