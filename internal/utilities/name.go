package utilities

func QualifiedName(name, namespace string) string {
	if namespace == "" {
		return name
	}

	return namespace + "/" + name
}
