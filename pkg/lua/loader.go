package lua

import (
	"fmt"
	"os"
	lua "github.com/yuin/gopher-lua"
)

type ConfigConexion struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

type BootstrapConfig struct {
	UIDB             ConfigConexion
	BusinessDB       ConfigConexion
	EntryPointQuery  string
	EntryPointViewID string
	LayoutQuery      string
}

func getStringField(tbl *lua.LTable, key string) string {
	val := tbl.RawGetString(key)
	if val == lua.LNil {
		return ""
	}
	return val.String()
}

func getIntField(tbl *lua.LTable, key string) int {
	val := tbl.RawGetString(key)
	if val == lua.LNil {
		return 0
	}
	if num, ok := val.(lua.LNumber); ok {
		return int(num)
	}
	// Fallback attempt to parse string as int
	var i int
	if _, err := fmt.Sscanf(val.String(), "%d", &i); err == nil {
		return i
	}
	return 0
}

func LoadConfig(path string) (*BootstrapConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", path)
	}

	L := lua.NewState()
	defer L.Close()

	if err := L.DoFile(path); err != nil {
		return nil, fmt.Errorf("failed to execute Lua config: %w", err)
	}

	val := L.GetGlobal("golemui_driver")
	if val.Type() != lua.LTTable {
		return nil, fmt.Errorf("golemui_driver table not found or invalid in Lua config")
	}
	tbl := val.(*lua.LTable)

	parseConexion := func(tableName string) (ConfigConexion, error) {
		subVal := tbl.RawGetString(tableName)
		if subVal.Type() != lua.LTTable {
			return ConfigConexion{}, fmt.Errorf("sub-table %s not found or invalid", tableName)
		}
		subTbl := subVal.(*lua.LTable)

		host := getStringField(subTbl, "Host")
		port := getIntField(subTbl, "Port")
		database := getStringField(subTbl, "Database")
		user := getStringField(subTbl, "User")
		password := getStringField(subTbl, "Password")

		// Validate required fields
		if host == "" || port == 0 || database == "" || user == "" {
			return ConfigConexion{}, fmt.Errorf("missing required connection fields in %s", tableName)
		}

		return ConfigConexion{
			Host:     host,
			Port:     port,
			Database: database,
			User:     user,
			Password: password,
		}, nil
	}

	uiDB, err := parseConexion("UIDB")
	if err != nil {
		return nil, err
	}

	bizDB, err := parseConexion("BusinessDB")
	if err != nil {
		return nil, err
	}

	entryPointQuery := getStringField(tbl, "EntryPointQuery")
	entryPointViewID := getStringField(tbl, "EntryPointViewID")
	layoutQuery := getStringField(tbl, "LayoutQuery")

	return &BootstrapConfig{
		UIDB:             uiDB,
		BusinessDB:       bizDB,
		EntryPointQuery:  entryPointQuery,
		EntryPointViewID: entryPointViewID,
		LayoutQuery:      layoutQuery,
	}, nil
}
