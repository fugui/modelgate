import os
import subprocess

moves = {
    # Domain
    "user": "domain/user",
    "apikey": "domain/apikey",
    "quota": "domain/quota",
    "usage": "domain/usage",
    "dashboard": "domain/dashboard",
    "admin": "domain/admin",
    
    # Gateway
    "proxy": "gateway/proxy",
    "openai": "gateway/openai",
    "anthropic": "gateway/anthropic",
    
    # Repository (Rename entity -> repository)
    "entity": "repository",
    
    # Infra
    "cache": "infra/cache",
    "concurrency": "infra/concurrency",
    "constants": "infra/constants",
    "db": "infra/db",
    "logger": "infra/logger",
    "middleware": "infra/middleware",
    "static": "infra/static",
    "utils": "infra/utils",
    "auth": "infra/auth",
}

# 1. Create directories
os.makedirs("internal/domain", exist_ok=True)
os.makedirs("internal/gateway", exist_ok=True)
os.makedirs("internal/infra", exist_ok=True)

# 2. Git mv
for src, dst in moves.items():
    src_path = f"internal/{src}"
    dst_path = f"internal/{dst}"
    if os.path.exists(src_path):
        print(f"Moving {src_path} to {dst_path}")
        subprocess.run(["git", "mv", src_path, dst_path], check=True)

# 3. Replace imports in all .go files
def replace_in_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    
    original = content
    for src, dst in moves.items():
        # Replace absolute package imports
        old_import = f'"modelgate/internal/{src}"'
        new_import = f'"modelgate/internal/{dst}"'
        content = content.replace(old_import, new_import)
        
        old_import_prefix = f'"modelgate/internal/{src}/'
        new_import_prefix = f'"modelgate/internal/{dst}/'
        content = content.replace(old_import_prefix, new_import_prefix)

    if content != original:
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"Updated imports in {filepath}")

for root, _, files in os.walk("."):
    if ".git" in root or "node_modules" in root:
        continue
    for file in files:
        if file.endswith(".go"):
            filepath = os.path.join(root, file)
            replace_in_file(filepath)

print("Refactoring done.")
