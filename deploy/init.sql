CREATE USER iqhost WITH PASSWORD 'iqhost-local';
CREATE DATABASE iqhost OWNER iqhost;
CREATE USER temporal WITH PASSWORD 'temporal-local' CREATEDB;
CREATE DATABASE temporal OWNER temporal;
CREATE DATABASE temporal_visibility OWNER temporal;
