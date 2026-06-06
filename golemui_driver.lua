golemui_driver = {
    UIDB = {
        Host = "localhost",
        Port = 5432,
        Database = "golemui_core",
        User = "golemui_core_engine",
        Password = "secret_password_for_ui"
    },
    BusinessDB = {
        Host = "localhost",
        Port = 5432,
        Database = "negocio_production",
        User = "golemui_render_engine",
        Password = "secret_password_for_business"
    },
    EntryPointViewID = "transacciones_list",
    LayoutQuery = "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"
}
