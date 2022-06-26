package record

// Shims for gradually refactoring the database.

// TODO Remove once AC-1598 is done.

type ShimValue interface {
	Record
	ValueReader
	ValueWriter
	NewCopy(Store) ShimValue
}

func (v *Value[T]) NewCopy(store Store) ShimValue {
	u := new(Value[T])
	u.logger = v.logger
	u.key = v.key
	u.name = v.name
	u.allowMissing = v.allowMissing

	u.store = store
	u.value = v.value.CopyAsInterface().(EncodableValue[T])
	u.value.setNew()
	return u
}
