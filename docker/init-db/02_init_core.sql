-- Inicialización de GolemUI Core en la base de datos golemui_core
CREATE SCHEMA IF NOT EXISTS golemui;

-- Catálogo de componentes de UI estándar
CREATE TABLE IF NOT EXISTS golemui.componentes (
    id VARCHAR(50) PRIMARY KEY,
    descripcion TEXT NOT NULL
);

INSERT INTO golemui.componentes (id, descripcion) VALUES
('click_button', 'Botón de ejecución transaccional'),
('text_input', 'Input de texto de una sola línea'),
('text_area', 'Input de texto multilínea'),
('numeric_stepper', 'Selector numérico con límites definidos'),
('barcode_reader', 'Control optimizado para entrada de escáneres rápidos'),
('data_grid', 'Grilla estructurada para visualización y selección de filas'),
('dropdown_select', 'Selector de opciones basado en claves foráneas'),
('date_picker', 'Selector gráfico de fechas calendarizadas'),
('checkbox_toggle', 'Selector booleano interactivo'),
('numeric_keypad', 'Teclado numérico táctil para ingreso rápido de datos')
ON CONFLICT (id) DO NOTHING;

-- Sistema de Diseño Semántico (Semantic Design Tokens)
CREATE TABLE IF NOT EXISTS golemui.estilos (
    id VARCHAR(50) PRIMARY KEY,
    color_fondo VARCHAR(7) NOT NULL,
    color_texto VARCHAR(7) NOT NULL,
    border_radius VARCHAR(20) NOT NULL DEFAULT 'smooth',
    font_size VARCHAR(20) NOT NULL DEFAULT 'medium',
    font_weight VARCHAR(20) NOT NULL DEFAULT 'normal'
);

INSERT INTO golemui.estilos (id, color_fondo, color_texto, border_radius, font_size, font_weight) VALUES
('primary_action', '#3498db', '#ffffff', 'smooth', 'medium', 'bold'),
('success', '#2ecc71', '#ffffff', 'smooth', 'medium', 'bold'),
('danger', '#e74c3c', '#ffffff', 'smooth', 'medium', 'bold'),
('input_standard', '#ffffff', '#2c3e50', 'sharp', 'small', 'normal'),
('sidebar_panel', '#2c3e50', '#ecf0f1', 'sharp', 'small', 'normal'),
('table_header', '#34495e', '#ffffff', 'sharp', 'small', 'bold'),
('table_cell', '#ffffff', '#2c3e50', 'sharp', 'small', 'normal')
ON CONFLICT (id) DO NOTHING;

-- Tabla central de overrides de interfaz (Capa 3)
CREATE TABLE IF NOT EXISTS golemui.mapeo_interfaz (
    origen_id VARCHAR(100) NOT NULL,
    columna_fisica VARCHAR(100) NOT NULL,
    component_ref VARCHAR(50) NOT NULL,
    label VARCHAR(150),
    placeholder VARCHAR(250),
    validation VARCHAR(250),
    PRIMARY KEY (origen_id, columna_fisica)
);

-- Tabla para almacenamiento temporal de formularios / borradores
CREATE TABLE IF NOT EXISTS golemui.sesion_borrador (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(100) NOT NULL,
    clave_campo VARCHAR(100) NOT NULL,
    valor_json JSONB NOT NULL,
    creado_en TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_golemui_borrador_sesion ON golemui.sesion_borrador(session_id);

-- Catálogo de pantallas de consulta del sistema
CREATE TABLE IF NOT EXISTS golemui.vistas_consulta (
    id VARCHAR(100) PRIMARY KEY,
    titulo VARCHAR(150) NOT NULL,
    origen_datos VARCHAR(150) NOT NULL,
    config_columnas JSONB NOT NULL,
    config_filtros JSONB NOT NULL
);

INSERT INTO golemui.vistas_consulta (id, titulo, origen_datos, config_columnas, config_filtros)
VALUES ('home', 'Home', 'SELECT 1',
  '{"area":"home_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"header","component_ref":"label","label":"Welcome to GolemUI Desktop Client"},{"area":"search_title","component_ref":"text_input","bind_to":"title","placeholder":"Filter by title"},{"area":"search_author","component_ref":"text_input","bind_to":"author","placeholder":"Filter by author"},{"area":"submit_btn","component_ref":"button","label":"Search","submit_action":"search"},{"area":"grid_area","component_ref":"data_grid","data_source":"SELECT * FROM transacciones WHERE descripcion LIKE $1 AND monto::text LIKE $2","filter_mode":"server","filter_keys":["title","author"]}]}'::jsonb,
  '[]'::jsonb)
ON CONFLICT (id) DO NOTHING;

INSERT INTO golemui.vistas_consulta (id, titulo, origen_datos, config_columnas, config_filtros)
VALUES ('transacciones_list', 'Listado de Transacciones', 'SELECT id, emp_cod, monto, status FROM public.transacciones WHERE emp_cod LIKE ''%'' || $1 || ''%'' AND status LIKE ''%'' || $2 || ''%''',
  '{"area":"root","component_ref":"container","layout":{"type":"grid","columns":["1fr"],"rows":["30px","50px","1fr"],"gap":"10"},"children":[{"area":"header","component_ref":"label","label":"Listado de Transacciones"},{"area":"filters_container","component_ref":"container","layout":{"type":"grid","columns":["250px","200px","120px","140px"],"rows":["40px"],"gap":"10"},"children":[{"area":"emp_cod_filter","component_ref":"text_input","placeholder":"Filtrar por Empresa (LIKE)","bind_to":"emp_cod"},{"area":"status_filter","component_ref":"text_input","placeholder":"Filtrar por Status (LIKE)","bind_to":"status"},{"area":"search_button","component_ref":"button","label":"Actualizar","submit_action":"search"},{"area":"goto_runner","component_ref":"button","label":"Consola SQL","submit_action":"navigate:query_runner"}]},{"area":"transactions_grid","component_ref":"data_grid","filter_mode":"server","data_source":"SELECT id, emp_cod, monto, status FROM public.transacciones WHERE emp_cod LIKE ''%'' || $1 || ''%'' AND status LIKE ''%'' || $2 || ''%''","filter_keys":["emp_cod","status"]}]}'::jsonb,
  '[]'::jsonb)
ON CONFLICT (id) DO NOTHING;

INSERT INTO golemui.vistas_consulta (id, titulo, origen_datos, config_columnas, config_filtros)
VALUES ('query_runner', 'Consola SQL', 'SELECT 1',
  '{"area":"root","component_ref":"container","layout":{"type":"grid","columns":["1fr"],"rows":["30px","100px","40px","1fr"],"gap":"10"},"children":[{"area":"header","component_ref":"label","label":"Consola de Consultas SQL"},{"area":"query_input","component_ref":"text_area","placeholder":"Escribí tu consulta SQL SELECT aquí...","bind_to":"sql_query","default_value":"SELECT id, emp_cod, monto, status FROM public.transacciones LIMIT 10"},{"area":"buttons_container","component_ref":"container","layout":{"type":"horizontal"},"children":[{"area":"execute_btn","component_ref":"button","label":"Ejecutar Query","submit_action":"search"},{"area":"back_btn","component_ref":"button","label":"Volver al Listado","submit_action":"navigate:transacciones_list"}]},{"area":"results_grid","component_ref":"data_grid","filter_mode":"server","data_source":"state:sql_query"}]}'::jsonb,
  '[]'::jsonb)
ON CONFLICT (id) DO NOTHING;


-- Vistas personalizadas guardadas por los usuarios
CREATE TABLE IF NOT EXISTS golemui.vistas_guardadas (
    id SERIAL PRIMARY KEY,
    vista_consulta_id VARCHAR(100) REFERENCES golemui.vistas_consulta(id),
    nombre_preset VARCHAR(150) NOT NULL,
    usuario_id VARCHAR(100) NOT NULL,
    filtros_aplicados JSONB NOT NULL,
    orden_columnas JSONB,
    es_predeterminada BOOLEAN DEFAULT FALSE
);
