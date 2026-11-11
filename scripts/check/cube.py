import re
from _lib import github, report
report("cube", [v for v in github("cube-js/cube") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
