package nlp

import (
	"strings"

	"installr/internal/store"
)

// domainSynonyms maps broad query keywords to lists of related tool names.
// When a query contains a keyword, the synonyms are appended to the query
// before embedding, improving recall for category searches.
var domainSynonyms = map[string][]string{
	"network": {
		"dns", "ssl", "tls", "tcp", "udp", "http", "https", "ftp", "ssh",
		"curl", "wget", "dig", "openssl", "netcat", "nc", "nmap", "ping",
		"traceroute", "ip", "socket", "proxy", "transfer", "protocol",
		"wireshark", "tcpdump", "whois", "host", "nslookup", "scp", "sftp",
		"telnet", "route", "iptables", "nftables", "bridge", "vlan", "bond",
		"wlan", "wifi", "ethernet", "mac", "arp", "icmp", "port",
		"firewall", "nat", "vpn", "openvpn", "wireguard", "stunnel",
		"haproxy", "nginx", "apache",
	},
	"python": {
		"python", "pip", "pypi", "venv", "virtualenv", "pyenv", "poetry",
		"uv", "conda", "ipython", "jupyter", "numpy", "pandas", "scipy",
		"matplotlib", "flask", "django", "fastapi", "pytest", "tox", "mypy",
		"black", "flake8", "pylint", "setuptools", "wheel", "pipenv",
		"coverage", "sphinx", "mkdocs", "boto3", "requests", "urllib",
		"httpx", "aiohttp", "asyncio", "twisted", "celery", "sqlalchemy",
		"alembic", "drf", "wagtail", "scrapy", "beautifulsoup", "selenium",
		"playwright", "pillow", "opencv", "tensorflow", "pytorch", "keras",
		"scikit", "xgboost", "lightgbm", "catboost", "nltk", "spacy",
		"transformers", "langchain", "pydantic", "uvicorn", "gunicorn",
		"redis", "kafka", "rabbitmq", "postgres", "mysql", "sqlite",
		"mongodb", "elasticsearch", "polars", "dask", "ray", "streamlit",
		"gradio", "plotly", "bokeh", "altair", "holoviews", "geopandas",
		"shapely", "fiona", "rasterio", "xarray", "netcdf4", "h5py", "zarr",
		"modin", "vaex", "ydata", "sweetviz", "autoviz", "great-expectations",
		"pandera", "cerberus", "marshmallow", "voluptuous", "schema",
		"validator", "attrs", "dataclasses", "typing", "pyright",
		"typeguard", "beartype", "starlette", "hypercorn", "daphne",
		"channels", "ninja", "tastypie", "quart", "sanic", "tornado",
		"litestar", "robyn", "falcon", "hug", "apistar", "responder",
		"molten", "clastic", "bottle", "cherrypy", "pyramid", "web2py",
		"wheezy", "webpy", "turbogears", "pylons", "zope", "groks",
		"bluebream",
	},
	"web": {
		"web", "http", "https", "html", "css", "javascript", "js", "browser",
		"server", "client", "api", "rest", "graphql", "websocket", "ajax",
		"curl", "wget", "nginx", "apache", "caddy", "traefik", "haproxy",
		"varnish", "squid", "nodejs", "node", "npm", "yarn", "pnpm", "bun",
		"deno", "react", "vue", "angular", "svelte", "solid", "nextjs",
		"nuxt", "gatsby", "astro", "remix", "express", "koa", "fastify",
		"hapi", "restify", "loopback", "nestjs", "django", "flask",
		"fastapi", "rails", "sinatra", "laravel", "symfony", "spring",
		"aspnet", "go", "gin", "echo", "fiber", "chi", "mux", "htmx",
		"jquery", "bootstrap", "tailwind", "sass", "less", "postcss",
		"webpack", "vite", "rollup", "parcel", "esbuild", "swc", "babel",
		"typescript", "ts", "jsx", "tsx", "playwright", "puppeteer",
		"selenium", "cypress", "jest", "mocha", "vitest", "karma",
		"jasmine", "nightwatch", "webdriver", "httpie", "postman",
		"insomnia", "swagger", "openapi", "grpc", "protobuf", "thrift",
		"jsonrpc", "soap", "xmlrpc", "aria2", "axel", "lftp", "rsync",
		"syncthing", "ytdlp", "ffmpeg", "imagemagick", "graphicsmagick",
		"pandoc", "marked", "markdown", "restructuredtext", "asciidoc",
		"hugo", "jekyll", "hexo", "docusaurus", "vitepress", "vuepress",
		"mkdocs", "sphinx", "gitbook", "bookstack", "wiki", "mediawiki",
		"dokuwiki", "confluence", "notion", "obsidian", "logseq", "trilium",
		"joplin", "zettlr", "remarkable", "typora", "marktext",
		"ghostwriter", "firefox", "chrome", "chromium", "brave", "vivaldi",
		"opera", "edge", "safari", "tor", "lynx", "links", "w3m",
		"elinks", "midori", "dillo", "netsurf", "qutebrowser", "falkon",
		"otter", "dooble", "arora", "konqueror", "rekonq",
	},
	"database": {
		"database", "db", "sql", "nosql", "postgres", "postgresql", "mysql",
		"mariadb", "sqlite", "mongodb", "redis", "cassandra", "couchdb",
		"dynamodb", "firestore", "bigquery", "snowflake", "clickhouse",
		"questdb", "timescaledb", "influxdb", "prometheus", "elasticsearch",
		"opensearch", "solr", "sphinx", "meilisearch", "typesense", "algolia",
		"neo4j", "arangodb", "orientdb", "tigergraph", "dgraph", "janusgraph",
		"gremlin", "cypher", "sparql", "rdf", "triplestore", "virtuoso",
		"blazegraph", "graphdb", "stardog", "memgraph", "kuzu", "duckdb",
		"litestream", "rqlite", "dqlite", "cockroachdb", "yugabytedb", "tidb",
		"vitess", "planetscale", "supabase", "neon", "citus", "greenplum",
		"redshift", "spectrum", "athena", "presto", "trino", "drill",
		"impala", "hive", "sparksql", "databricks", "delta", "iceberg",
		"hudi", "avro", "parquet", "orc", "protobuf", "thrift", "csv",
		"tsv", "json", "xml", "yaml", "toml", "ini", "config", "schema",
		"migration", "flyway", "liquibase", "alembic", "dbmate",
		"golang-migrate", "migrate", "goose", "sqlc", "prisma", "typeorm",
		"sequelize", "sqlalchemy", "peewee", "tortoise", "orm", "odm",
		"knex", "objection", "bookshelf", "waterline", "camo", "ottoman",
		"mikroorm", "drizzle", "kysely", "zapatos", "slonik", "pg",
		"pgpromise", "nodepostgres", "mysql2", "better-sqlite3", "sqlite3",
		"tedious", "mssql", "oracledb", "db2", "ibm_db", "couchbase",
		"pouchdb", "rxdb", "realm", "watermelon", "dexie", "localforage",
		"lovefield", "lokijs", "nedb", "tingodb", "tingo", "alasql",
		"sqljs", "absurd-sql", "electric-sql", "powersync", "sqlitevfs",
		"sqlcipher", "sqlar", "fossil", "litefs",
	},
	"container": {
		"docker", "container", "podman", "kubernetes", "k8s", "kubectl",
		"helm", "k3s", "minikube", "kind", "microk8s", "rancher", "openshift",
		"nomad", "consul", "vault", "terraform", "pulumi", "ansible",
		"chef", "puppet", "salt", "vagrant", "packer", "jenkins", "gitlab",
		"github", "actions", "circleci", "travisci", "drone", "argo",
		"tekton", "spinnaker", "flux", "flagger", "istio", "linkerd",
		"envoy", "traefik", "nginx", "haproxy", "caddy", "varnish",
		"squid", "prometheus", "grafana", "loki", "thanos", "cortex",
		"jaeger", "zipkin", "otel", "opentelemetry", "fluentd", "fluentbit",
		"vector", "logstash", "filebeat", "metricbeat", "heartbeat",
		"auditbeat", "packetbeat", "apm", "newrelic", "datadog", "sentry",
		"bugsnag", "rollbar", "honeycomb", "lightstep", "signoz",
		"hyperdx", "plausible", "umami", "fathom", "matomo", "piwik",
		"cloudwatch", "stackdriver", "azuremonitor", "dynatrace", "appdynamics",
		"instana", "splunk", "elasticsearch", "kibana", "logstash", "beats",
		"wazuh", "ossec", "suricata", "snort", "zeek", "bro", "nmap",
		"masscan", "zmap", "openvas", "nessus", "qualys", "rapid7",
		"metasploit", "burp", "owasp", "zap", "nikto", "sqlmap", "dirb",
		"gobuster", "feroxbuster", "rustscan", "naabu", "httpx", "katana",
		"nuclei", "subfinder", "amass", "assetfinder", "findomain",
		"chaos", "shuffledns", "dnsx", "alterx", "mapcidr", "asnmap",
		"cloudlist", "proxify", "teler", "notify", "interactsh",
	},
	"security": {
		"security", "crypto", "cryptography", "cipher", "encrypt", "decrypt",
		"ssl", "tls", "openssl", "gnutls", "libressl", "boringssl", "wolfssl",
		"mbedtls", "nss", "gpg", "pgp", "openpgp", "gnupg", "gpgme",
		"keybase", "keyczar", "tink", "libsodium", "nacl", "argon2",
		"bcrypt", "scrypt", "pbkdf2", "hkdf", "hmac", "sha", "md5",
		"blake", "keccak", "sha3", "sha256", "sha512", "aes", "rsa",
		"ecdsa", "ed25519", "curve25519", "x25519", "x448", "ecdh", "ecies",
		"dh", "dsa", "ec", "ecc", "pq", "post-quantum", "lattice",
		"kyber", "dilithium", "falcon", "sphincs", "sike", "ntru", "nist",
		"fips", "commoncrypto", "cryptlib", "botan", "cryptopp", "libgcrypt",
		"nettle", "tomcrypt", "bearssl", "ring", "rustls", "openssl",
		"s2n", "aws-lc", "cert", "certificate",
		"ca", "pki", "x509", "acme", "letsencrypt", "certbot", "lego",
		"acme.sh", "dehydrated", "cert-manager", "step-ca", "smallstep",
		"vault", "consul", "boundary",
		"openvpn", "wireguard", "stunnel", "haproxy", "nginx", "apache",
		"firewall", "iptables", "nftables", "pf", "ipfw", "shorewall",
		"ufw", "fail2ban", "crowdsec", "suricata", "snort", "zeek", "bro",
		"ossec", "wazuh", "aide", "samhain", "tripwire", "osquery",
		"fleetdm", "kolide", "velociraptor", "grr", "yara", "clamav",
		"rkhunter", "chkrootkit", "lynis", "tiger", "bastille", "secdev",
		"apparmor", "selinux", "tomoyo", "smack", "ima", "evm", "tpm",
		"tss", "tpm2", "fido", "u2f", "webauthn", "passkey", "yubikey",
		"nitrokey", "solokey", "onlykey", "hyperfido", "biometric",
		"fingerprint", "face", "iris", "voice", "keystroke", "behavioral",
		"anomaly", "siem", "splunk", "elasticsearch", "logstash", "kibana",
		"wazuh", "ossec", "graylog", "loki", "grafana", "prometheus",
		"thanos", "cortex", "victoriametrics", "mimir", "tempo", "jaeger",
		"zipkin", "otel", "opentelemetry", "signoz", "hyperdx", "honeycomb",
		"lightstep", "newrelic", "datadog", "sentry", "bugsnag", "rollbar",
		"instana", "dynatrace", "appdynamics", "scouter", "pinpoint",
		"skywalking", "cat", "zipkin", "jaeger", "tempo", "otel",
		"nmap", "masscan", "zmap", "openvas", "nessus", "qualys", "rapid7",
		"metasploit", "burp", "owasp", "zap", "nikto", "sqlmap", "dirb",
		"gobuster", "feroxbuster", "rustscan", "naabu", "httpx", "katana",
		"nuclei", "subfinder", "amass", "assetfinder", "findomain",
		"chaos", "shuffledns", "dnsx", "alterx", "mapcidr", "asnmap",
		"cloudlist", "proxify", "teler", "notify", "interactsh",
	},
	"git": {
		"git", "github", "gitlab", "gitea", "gogs", "bitbucket", "sourcehut",
		"srht", "cgit", "stagit", "gitweb", "codeberg",
		"forgejo", "gitbucket", "onedev", "agit", "gitolite",
		"svn", "subversion", "mercurial", "hg", "bazaar", "bzr",
		"darcs", "fossil", "cvs", "rcs", "sccs", "tfs", "vss", "clearcase",
		"perforce", "p4", "tig", "lazygit", "gitui", "gitoxide",
		"gitbutler", "gitkraken", "sourcetree", "tower", "fork", "gitup",
		"gitx", "gitg", "git-cola", "gitty", "git2", "libgit2", "git2go",
		"rugged", "dulwich", "pygit2", "gitpython", "go-git", "git2go",
		"git2-rs", "git2", "commit", "branch", "merge", "rebase", "cherry",
		"pick", "blame", "bisect", "stash", "tag", "reflog", "hook",
		"pre-commit", "husky", "lint-staged", "commitlint", "semantic",
		"release", "standard", "conventional", "changelog", "git-cliff",
		"git-chglog", "github-changelog-generator", "gitmoji", "gitmoji-cli",
		"cz", "commitizen", "git-cz",
	},
	"build": {
		"build", "compiler", "compile", "make", "cmake", "meson", "ninja", "bazel",
		"buck", "gradle", "maven", "ant", "sbt", "cargo", "rustc", "gcc",
		"clang", "llvm", "lld", "ld", "ar", "as", "cc", "c++", "cpp",
		"fortran", "gfortran", "go", "golang", "tsc", "typescript",
		"esbuild", "swc", "babel", "webpack", "vite", "rollup", "parcel",
		"turbo", "turbopack", "turborepo", "nx", "lerna", "yarn", "pnpm",
		"npm", "bun", "deno", "node", "nodejs", "python", "pip", "poetry",
		"uv", "conda", "mamba", "pipenv", "virtualenv", "pyenv", "ruby",
		"gem", "bundler", "rbenv", "rvm", "chruby", "rust", "cargo",
		"rustup", "crates", "java", "maven", "gradle", "ant", "sbt",
		"leiningen", "clojure", "lein", "boot", "shadow", "cljs",
		"clojurescript", "kotlin", "kscript", "kobalt", "scala", "sbt",
		"mill", "ammonite", "coursier", "cs", "haskell", "cabal", "stack",
		"ghc", "ghcup", "ghci", "runghc", "ghc-pkg", "cabal-install",
		"elm", "elm-format", "elm-review", "elm-test", "elm-live",
		"elm-make", "elm-package", "elm-repl", "elm-reactor", "elm-analyse",
	},
	"editor": {
		"editor", "ide", "vim", "neovim", "nvim", "emacs", "nano", "micro",
		"helix", "hx", "kakoune", "kak", "sublime", "vscode", "code",
		"vscodium", "cursor", "zed", "fleet", "intellij", "pycharm", "goland",
		"rubymine", "phpstorm", "webstorm", "clion", "rider", "datagrip",
		"appcode", "studio", "android", "xcode", "eclipse", "netbeans",
		"jbuilder", "jdeveloper", "codelite", "codeblocks", "geany", "mousepad",
		"leafpad", "gedit", "kate", "kwrite", "notepad", "notepad++", "npp",
		"textmate", "bbedit", "coda", "nova", "caret", "typora", "marktext",
		"ghostwriter", "abiword", "libreoffice", "openoffice", "onlyoffice",
		"wps", "softmaker", "freeoffice", "calligra", "koffice", "kword",
		"kspread", "kpresenter", "kdiagram", "kexi", "krita", "inkscape",
		"gimp", "darktable", "rawtherapee", "digikam", "shotwell", "f-spot",
		"picasa", "gthumb", "eog", "feh", "sxiv", "nsxiv", "imv", "mpv",
		"vlc", "mplayer", "smplayer", "kmplayer", "totem", "parole", "xine",
		"audacious", "clementine", "amarok", "rhythmbox", "banshee", "exaile",
		"quodlibet", "deadbeef", "cmus", "moc", "mpc", "mpd", "ncmpcpp",
		"ncspot", "spotify", "spotifyd", "spotify-tui", "spt", "tizonia",
		"mopidy", "mopidy-spotify", "mopidy-mpd", "mopidy-local", "mopidy-scrobbler",
	},
	"test": {
		"test", "testing", "pytest", "jest", "mocha", "vitest", "karma",
		"jasmine", "nightwatch", "cypress", "playwright", "puppeteer",
		"selenium", "webdriver", "appium", "calabash", "cucumber", "gherkin",
		"behave", "lettuce", "radish", "pytest-bdd", "pytest-django",
		"pytest-flask", "pytest-asyncio", "pytest-cov", "pytest-xdist",
		"pytest-benchmark", "pytest-mock", "pytest-freezegun", "pytest-time",
		"pytest-timeout", "pytest-repeat", "pytest-randomly", "pytest-rerun",
		"pytest-order", "pytest-dependency", "pytest-instafail", "pytest-sugar",
		"pytest-picked", "pytest-testmon", "pytest-watch", "pytest-watcher",
		"pytest-github-actions", "pytest-gitlab", "pytest-jenkins", "pytest-circleci",
		"pytest-travisci", "pytest-drone", "pytest-argo", "pytest-tekton",
		"pytest-spinnaker", "pytest-flux", "pytest-flagger", "pytest-istio",
		"pytest-linkerd", "pytest-envoy", "pytest-traefik", "pytest-nginx",
		"pytest-haproxy", "pytest-caddy", "pytest-varnish", "pytest-squid",
		"pytest-prometheus", "pytest-grafana", "pytest-loki", "pytest-thanos",
		"pytest-cortex", "pytest-jaeger", "pytest-zipkin", "pytest-otel",
		"pytest-opentelemetry", "pytest-fluentd", "pytest-fluentbit",
		"pytest-vector", "pytest-logstash", "pytest-filebeat", "pytest-metricbeat",
		"pytest-heartbeat", "pytest-auditbeat", "pytest-packetbeat", "pytest-apm",
		"pytest-newrelic", "pytest-datadog", "pytest-sentry", "pytest-bugsnag",
		"pytest-rollbar", "pytest-honeycomb", "pytest-lightstep", "pytest-signoz",
		"pytest-hyperdx", "pytest-plausible", "pytest-umami", "pytest-fathom",
		"pytest-matomo", "pytest-piwik", "pytest-cloudwatch", "pytest-stackdriver",
		"pytest-azuremonitor", "pytest-dynatrace", "pytest-appdynamics",
		"pytest-instana", "pytest-splunk", "pytest-elasticsearch", "pytest-kibana",
		"pytest-logstash", "pytest-beats", "pytest-wazuh", "pytest-ossec",
		"pytest-suricata", "pytest-snort", "pytest-zeek", "pytest-bro",
		"pytest-nmap", "pytest-masscan", "pytest-zmap", "pytest-openvas",
		"pytest-nessus", "pytest-qualys", "pytest-rapid7", "pytest-metasploit",
		"pytest-burp", "pytest-owasp", "pytest-zap", "pytest-nikto", "pytest-sqlmap",
		"pytest-dirb", "pytest-gobuster", "pytest-feroxbuster", "pytest-rustscan",
		"pytest-naabu", "pytest-httpx", "pytest-katana", "pytest-nuclei",
		"pytest-subfinder", "pytest-amass", "pytest-assetfinder", "pytest-findomain",
		"pytest-chaos", "pytest-shuffledns", "pytest-dnsx", "pytest-alterx",
		"pytest-mapcidr", "pytest-asnmap", "pytest-cloudlist", "pytest-proxify",
		"pytest-teler", "pytest-notify", "pytest-interactsh",
	},
}

// ExpandQuery adds domain-specific synonyms to the query to improve semantic
// recall for broad categories.
func ExpandQuery(query string) string {
	q := strings.ToLower(query)
	var extras []string
	for keyword, synonyms := range domainSynonyms {
		if strings.Contains(q, keyword) {
			extras = append(extras, synonyms...)
		}
	}
	if len(extras) > 0 {
		return query + " " + strings.Join(extras, " ")
	}
	return query
}

// KeywordScore returns a boost (0-1+) for packages whose names or descriptions
// match known domain terms implied by the query.  This is a lightweight hybrid
// component that guarantees e.g. "dig" appears when you ask for "networking tools".
func KeywordScore(query string, pkg store.Package) float64 {
	q := strings.ToLower(query)
	name := strings.ToLower(pkg.Name)
	desc := strings.ToLower(pkg.Description)

	var score float64

	// Exact or partial name match is strong
	if name == q {
		score += 0.5
	}
	if strings.Contains(name, q) || strings.Contains(q, name) {
		score += 0.3
	}

	// Check each domain
	for keyword, synonyms := range domainSynonyms {
		if !strings.Contains(q, keyword) {
			continue
		}
		for _, term := range synonyms {
			// Skip very short terms to avoid false positives
			// (e.g. "ip" matching "pip", "go" matching "cargo").
			if len(term) < 3 {
				continue
			}
			if strings.Contains(name, term) {
				score += 0.25
			}
			if strings.Contains(desc, term) {
				score += 0.1
			}
		}
	}

	return score
}
