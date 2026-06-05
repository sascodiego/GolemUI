#!/bin/bash
set -e

echo "Inicializando Base de Datos de Negocio (negocio_production)..."

# 1. Crear la tabla transacciones en negocio_production
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "negocio_production" <<-EOSQL
    CREATE TABLE IF NOT EXISTS public.transacciones (
        id INT PRIMARY KEY,
        token_nro UUID,
        emp_cod VARCHAR(50),
        emp_hash VARCHAR(100),
        term_cod VARCHAR(50),
        monto NUMERIC(15,2),
        moneda_iso VARCHAR(10),
        operacion VARCHAR(10),
        status VARCHAR(50),
        emisor_id INT,
        factura_consumidor_final BOOLEAN,
        factura_monto NUMERIC(15,2),
        factura_monto_gravado NUMERIC(15,2),
        factura_monto_iva NUMERIC(15,2),
        factura_nro VARCHAR(50),
        monto_cash_back NUMERIC(15,2),
        monto_propina NUMERIC(15,2),
        multi_emp INT,
        tarjeta_alimentacion BOOLEAN,
        tarjeta_id INT,
        tarjeta_tipo VARCHAR(10),
        ticket_original INT,
        aprobada BOOLEAN,
        cod_resp_adq VARCHAR(10),
        es_offline BOOLEAN,
        lote INT,
        msg_respuesta TEXT,
        nro_autorizacion VARCHAR(50),
        resp_codigo INT,
        resp_estado_avance VARCHAR(100),
        resp_mensaje_error TEXT,
        resp_token_segundos_reconsultar INT,
        resp_transaccion_finalizada BOOLEAN,
        ticket INT,
        transaccion_id_getnet BIGINT,
        raw_response TEXT,
        comportamiento_json JSONB,
        configuracion_json JSONB,
        extendida_json JSONB,
        datos_transaccion_json JSONB,
        voucher_json JSONB,
        created_at TIMESTAMP,
        updated_at TIMESTAMP
    );
EOSQL

# 2. Ingestar los datos de prueba de transactions.json si existe
if [ -f /data/transactions.json ]; then
    echo "Cargando datos de prueba desde /data/transactions.json..."
    JSON_CONTENT=$(cat /data/transactions.json)
    
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "negocio_production" <<-EOSQL
        CREATE TEMP TABLE tmp_json (val jsonb);
        INSERT INTO tmp_json VALUES (\$\$$JSON_CONTENT\$\$::jsonb);
        INSERT INTO public.transacciones
        SELECT * FROM jsonb_populate_recordset(NULL::public.transacciones, (SELECT val FROM tmp_json))
        ON CONFLICT (id) DO NOTHING;
EOSQL
    echo "Carga de transacciones finalizada exitosamente."
else
    echo "ADVERTENCIA: No se encontró el archivo /data/transactions.json para la ingesta de prueba."
fi

# 3. Otorgar permisos al usuario de renderizado (golemui_render_engine) sobre las tablas de negocio
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "negocio_production" <<-EOSQL
    GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO golemui_render_engine;
    GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO golemui_render_engine;
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO golemui_render_engine;
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO golemui_render_engine;
EOSQL

