package testdriver

const EnvVar = "HIVE_LEAN_TEST_DRIVER"

func IsEnabled(value string) bool {
	switch value {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	}
	return false
}
