package typegen

type RecordType int

type Record interface {
	Type() RecordType
	GetContainer() *ContainerRecord
	GetParameters() []*Field
	FullName() string
	GetName() string
}

type ValueRecord interface {
	Record
	Wrapped() bool
	IsSet() bool
	IsCounted() bool
	IsPointer() bool
	IsUnion() bool
	GetDataType() FieldType
}

func (t RecordType) IsContainer() bool { return t == RecordTypeContainer }
func (t RecordType) IsChain() bool     { return t == RecordTypeChain }
func (t RecordType) IsState() bool     { return t == RecordTypeState }
func (t RecordType) IsIndex() bool     { return t == RecordTypeIndex }
func (t RecordType) IsOther() bool     { return t == RecordTypeOther }

func (r *ContainerRecord) GetName() string { return r.Name }
func (r *ChainRecord) GetName() string     { return r.Name }
func (r *StateRecord) GetName() string     { return r.Name }
func (r *IndexRecord) GetName() string     { return r.Name }
func (r *OtherRecord) GetName() string     { return r.Name }

func (r *ContainerRecord) GetParameters() []*Field { return r.Parameters }
func (r *ChainRecord) GetParameters() []*Field     { return r.Parameters }
func (r *StateRecord) GetParameters() []*Field     { return r.Parameters }
func (r *IndexRecord) GetParameters() []*Field     { return r.Parameters }
func (r *OtherRecord) GetParameters() []*Field     { return r.Parameters }

func (r *ContainerRecord) GetContainer() *ContainerRecord { return r.Container }
func (r *ChainRecord) GetContainer() *ContainerRecord     { return r.Container }
func (r *StateRecord) GetContainer() *ContainerRecord     { return r.Container }
func (r *IndexRecord) GetContainer() *ContainerRecord     { return r.Container }
func (r *OtherRecord) GetContainer() *ContainerRecord     { return r.Container }

func (r *ContainerRecord) FullName() string { return recordFullName(r) }
func (r *ChainRecord) FullName() string     { return recordFullName(r) }
func (r *StateRecord) FullName() string     { return recordFullName(r) }
func (r *IndexRecord) FullName() string     { return recordFullName(r) }
func (r *OtherRecord) FullName() string     { return recordFullName(r) }

func (r *ContainerRecord) Root() bool { return r.Container == nil }

func (r *ContainerRecord) HasChains() bool {
	for _, p := range r.Parts {
		switch p := p.(type) {
		case *ContainerRecord:
			if p.HasChains() {
				return true
			}
		case *ChainRecord:
			return true
		}
	}
	return false
}

func (r *StateRecord) Wrapped() bool { return r.DataType.Code != TypeCodeUnknown }
func (r *IndexRecord) Wrapped() bool { return r.DataType.Code != TypeCodeUnknown }

func (r *StateRecord) IsSet() bool { return r.Set }
func (r *IndexRecord) IsSet() bool { return r.Set }

func (r *StateRecord) IsUnion() bool { return r.Union }
func (r *IndexRecord) IsUnion() bool { return r.Union }

func (r *StateRecord) IsCounted() bool { return r.Counted }
func (r *IndexRecord) IsCounted() bool { return r.Counted }

func (r *StateRecord) IsPointer() bool { return r.Pointer }
func (r *IndexRecord) IsPointer() bool { return r.Pointer }

func (r *StateRecord) GetDataType() FieldType { return r.DataType }
func (r *IndexRecord) GetDataType() FieldType { return r.DataType }

func recordFullName(r Record) string {
	if r.GetContainer() == nil || r.GetContainer().Container == nil {
		return r.GetName()
	}
	return recordFullName(r.GetContainer()) + r.GetName()
}
