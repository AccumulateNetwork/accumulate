# Heirarchical Caching Data Model

The goals of the record package and the model generator are:

1. Explicitly define a conceptual data model
2. Generate types that mirror the conceptual data model
3. Move caching into the data model

## Conceptual Data Model

### Entities

An entity is a record that is composed of a set of attributes. There are two
basic categories of attributes: complex and simple. A complex attribute is one
that itself has attributes, such as an entity or a chain. A simple attribute
represents a single value or a simple collection of values. Since an attribute
can be an entity, which itself has attributes, the data model is heirarchical.

### Simple Attributes

Simple attributes are state or index attributes. A state attribute is considered
to be part of the proper state of the entity, whereas index attributes exist
soley to index other records.

Simple attributes can be atomic values, sets, or lists. Atomic values may
themselves be structured, but they are persisted atomically so they are
considered to be a single value by the CDM. Sets and lists are ordered and
unordered collections of values, respectively.

### Parameterized Attributes

An attribute of an entity may be parameterized. A parameterized attribute is
effectively a map or dictionary where the key is a tuple of the parameters. For
example, the root entity in a personnel database may have an attribute, Person,
with the a string parameter, Name. accessed as `model.Person(name)`.

## Implementation

The generator produces a struct type for every entity, with a method for every
attribute. The return type for a complex attribute is the corresponding entity
struct type. The return type for a simple attribute is one of the simple
attribute types defined in the record package. Parameterized attributes are
backed by a map and single-valued attributes are backed by a simple field. The
first time an attribute is accessed, an object of the appropriate type is
created. Subsequent accesses will return the same object.

## Example

The following is an example of a data model that could be used to track
personnel.

```yaml
- name: Model
  type: entity
  root: true
  attributes:
    - name: Person
      type: entity
      parameters:
        - name: Surname
          type: string
        - name: GivenName
          type: string
      attributes:
        - name: State
          type: state
          dataType: PersonState
          pointer: true
        - name: Dependents
          type: state
          dataType: string
          collection: set
```

```go
type Model struct{...}

func (*Model) Person(surname, givenName string) *ModelPerson

type ModelPerson struct{...}

func (*ModelPerson) State() *record.Value[*PersonState]
func (*ModelPerson) Dependents() *record.Set[string]
```