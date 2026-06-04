#!/bin/bash
set -e

# Crear el segundo usuario y la segunda base de datos (de negocio)
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE USER golemui_render_engine WITH PASSWORD 'secret_password_for_business';
    CREATE DATABASE negocio_production;
    GRANT ALL PRIVILEGES ON DATABASE negocio_production TO golemui_render_engine;
EOSQL
