# pgAdmin settings for the Quenchworks image: server mode, data in /var/lib/pgadmin, the
# PostgreSQL 18 client tools, no upgrade check. Any PGADMIN_CONFIG_<NAME> environment variable
# sets <NAME>; its value is a Python literal ('True', '10', "'text'"), else taken as a string.
import ast
import os

SERVER_MODE = True
DATA_DIR = os.environ.get('PGADMIN_DATA_DIR', '/var/lib/pgadmin')
UPGRADE_CHECK_ENABLED = False
DEFAULT_BINARY_PATHS = {'pg': '/usr/bin', 'pg-18': '/usr/bin'}

for _k, _v in os.environ.items():
    if _k.startswith('PGADMIN_CONFIG_'):
        try:
            globals()[_k[len('PGADMIN_CONFIG_'):]] = ast.literal_eval(_v)
        except (ValueError, SyntaxError):
            globals()[_k[len('PGADMIN_CONFIG_'):]] = _v
