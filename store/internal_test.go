package store

func TestingIsNoneType[T any]() bool {
	return isNoneType[T]()
}
