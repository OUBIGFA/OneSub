$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$python = Join-Path $root '.local/skill-tools-venv/Scripts/python.exe'
if (-not (Test-Path $python)) {
    & python -m venv (Join-Path $root '.local/skill-tools-venv')
    if ($LASTEXITCODE -ne 0) { throw 'Skill build environment creation failed' }
}
& $python -m pip install --disable-pip-version-check -r (Join-Path $root 'workers/skill-tools/requirements-build.txt')
if ($LASTEXITCODE -ne 0) { throw 'Skill build dependencies failed' }
$source = Join-Path $root 'third_party/skills/srt-subtitle-translator'
& $python -m PyInstaller --noconfirm --clean --onedir --name OneSub.SkillTools --paths (Join-Path $source 'scripts') --distpath (Join-Path $root 'build/output/skill-stage') --workpath (Join-Path $root 'workers/skill-tools/build') --specpath (Join-Path $root 'workers/skill-tools/build') (Join-Path $root 'workers/skill-tools/main.py')
if ($LASTEXITCODE -ne 0) { throw 'Skill worker build failed' }
$out = Join-Path $root 'build/output/skill-tools'
New-Item -ItemType Directory -Path $out -Force | Out-Null
Copy-Item (Join-Path $root 'build/output/skill-stage/OneSub.SkillTools/*') $out -Recurse -Force
Copy-Item (Join-Path $source 'config') $out -Recurse -Force
