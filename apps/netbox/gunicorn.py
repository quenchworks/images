# gunicorn settings for the Quenchworks NetBox image.
import os

bind = '0.0.0.0:' + os.environ.get('NETBOX_PORT', '8080')
workers = int(os.environ.get('GUNICORN_WORKERS', '3'))
threads = int(os.environ.get('GUNICORN_THREADS', '3'))
timeout = int(os.environ.get('GUNICORN_TIMEOUT', '120'))
max_requests = 5000
max_requests_jitter = 500
worker_tmp_dir = '/tmp'   # the root filesystem may be read-only
accesslog = '-'
