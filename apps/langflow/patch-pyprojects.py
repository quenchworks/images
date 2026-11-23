# Applied to the Langflow source tree before the lock is made and before the build (images/apps/langflow).
import glob, re
for p in glob.glob("**/pyproject.toml", recursive=True):
    s = o = open(p).read()
    s = s.replace("pyarrow>=26.0.1", "pyarrow>=25.0.1")              # Wolfi's build links system OpenSSL/curl
    s = s.replace("sqlalchemy[postgresql_psycopg2binary]", "sqlalchemy[postgresql]")  # psycopg2 from Wolfi
    s = s.replace('"pydantic>=2.13.0,<2.14"', '"pydantic>=2.13.0,<2.15"')   # pydantic-core 2.50 (Wolfi, pyo3 0.29)
    s = s.replace('"pydantic>=2.0.0,<2.14"', '"pydantic>=2.0.0,<2.15"')
    if p == "pyproject.toml":
        s = re.sub(r'\n    "lfx-ibm[^"]*",', "", s)                     # ibm_db: DB2 client, cannot be made clean
    if s != o:
        open(p, "w").write(s); print("patched", p)
