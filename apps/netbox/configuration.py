# NetBox configuration from the environment (Quenchworks image). Each *_PASSWORD, SECRET_KEY
# and API_TOKEN_PEPPER also reads <NAME>_FILE. More settings: a Python file at
# NETBOX_EXTRA_CONFIG (default /etc/netbox/extra.py), executed after this one.
import os


def _env(name, default=None):
    return os.environ.get(name, default)


def _secret(name, default=None):
    path = os.environ.get(name + '_FILE')
    if path:
        with open(path) as f:
            return f.read().strip()
    return os.environ.get(name, default)


def _bool(name, default='false'):
    return _env(name, default).lower() in ('1', 'true', 'yes')


ALLOWED_HOSTS = _env('ALLOWED_HOSTS', '*').split()
SECRET_KEY = _secret('SECRET_KEY')

DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.postgresql',
        'NAME': _env('DB_NAME', 'netbox'),
        'USER': _env('DB_USER', 'netbox'),
        'PASSWORD': _secret('DB_PASSWORD', ''),
        'HOST': _env('DB_HOST', 'localhost'),
        'PORT': _env('DB_PORT', '5432'),
        'CONN_MAX_AGE': int(_env('DB_CONN_MAX_AGE', '300')),
        'OPTIONS': {'sslmode': _env('DB_SSLMODE', 'prefer')},
    }
}


def _redis(db):
    return {
        'HOST': _env('REDIS_HOST', 'localhost'),
        'PORT': int(_env('REDIS_PORT', '6379')),
        'USERNAME': _env('REDIS_USERNAME', ''),
        'PASSWORD': _secret('REDIS_PASSWORD', ''),
        'DATABASE': db,
        'SSL': _bool('REDIS_SSL'),
    }


REDIS = {
    'tasks': _redis(int(_env('REDIS_TASKS_DATABASE', '0'))),
    'caching': _redis(int(_env('REDIS_CACHE_DATABASE', '1'))),
}

_pepper = _secret('API_TOKEN_PEPPER')
if _pepper:
    API_TOKEN_PEPPERS = {1: _pepper}

MEDIA_ROOT = _env('MEDIA_ROOT', '/opt/netbox/media')
TIME_ZONE = _env('TIME_ZONE', 'UTC')
LOGIN_REQUIRED = _bool('LOGIN_REQUIRED', 'true')
METRICS_ENABLED = _bool('METRICS_ENABLED')

_extra = _env('NETBOX_EXTRA_CONFIG', '/etc/netbox/extra.py')
if os.path.isfile(_extra):
    with open(_extra) as f:
        exec(compile(f.read(), _extra, 'exec'))
