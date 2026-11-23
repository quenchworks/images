# gunicorn entry point: NetBox's WSGI app with whitenoise serving the collected /static/
# (NetBox does not serve static files itself outside DEBUG).
from django.conf import settings
from whitenoise import WhiteNoise

from netbox.wsgi import application as _netbox

application = WhiteNoise(_netbox, root=settings.STATIC_ROOT, prefix=settings.STATIC_URL.lstrip('/'))
