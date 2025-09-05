package response

type Jsonable interface {
	ToJSONString() (string, error)
}
