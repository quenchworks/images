#!/usr/bin/env python3
# mosquitto: mosquitto.org; versions = GitHub tags vX.Y.Z (eclipse/mosquitto).
# Drop rc tags (v2.1.0rc3).
import re
from _lib import github_tags, report
report("mosquitto", github_tags("eclipse/mosquitto"), keep=lambda t: bool(re.fullmatch(r"\d+\.\d+\.\d+", t)))
