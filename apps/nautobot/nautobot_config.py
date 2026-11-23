# Nautobot configuration for the Quenchworks image. Nautobot's own settings already read the
# environment (NAUTOBOT_DB_*, NAUTOBOT_REDIS_*, NAUTOBOT_SECRET_KEY, NAUTOBOT_ALLOWED_HOSTS,
# NAUTOBOT_CELERY_*...); this file moves the writable paths under /var/lib/nautobot (the root
# filesystem may be read-only) and loads NAUTOBOT_EXTRA_CONFIG (default
# /etc/nautobot/extra.py) for settings with no variable (PLUGINS and the like).
import os

from nautobot.core.settings import *  # noqa: F401,F403

_data = os.getenv("NAUTOBOT_DATA_DIR", "/var/lib/nautobot")
MEDIA_ROOT = os.getenv("NAUTOBOT_MEDIA_ROOT", os.path.join(_data, "media"))
GIT_ROOT = os.getenv("NAUTOBOT_GIT_ROOT", os.path.join(_data, "git"))
JOBS_ROOT = os.getenv("NAUTOBOT_JOBS_ROOT", os.path.join(_data, "jobs"))

_extra = os.getenv("NAUTOBOT_EXTRA_CONFIG", "/etc/nautobot/extra.py")
if os.path.isfile(_extra):
    with open(_extra) as f:
        exec(compile(f.read(), _extra, "exec"))
