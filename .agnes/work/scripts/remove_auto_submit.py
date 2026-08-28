#!/usr/bin/env python3
"""Remove auto-submit from AddAccountDialog.tsx"""

import re
from pathlib import Path

file_path = Path(r"src/renderer/src/components/providers/AddAccountDialog.tsx")
content = file_path.read_text(encoding="utf-8")

# Remove the auto-submit block
old_pattern = r'''        // Auto-submit account creation after successful OAuth login\s*\n\s*setTimeout\(\(\) => \{\s*\n\s*handleSubmit\(\)\s*\n\s*\.then\(\(\) => \{\s*\n\s*console\.log\('\[AddAccountDialog\] OAuth account created successfully'\)\s*\n\s*\}\)\s*\n\s*\.catch\(\(error\) => \{\s*\n\s*console\.error\('\[AddAccountDialog\] Auto-submit failed:', error\)\s*\n\s*setOAuthStatus\(error instanceof Error \? error\.message : t\('providers\.saveFailed'\)\)\s*\n\s*\}\)\s*\n\s*\}, 500\)'''

new_content = re.sub(old_pattern, '', content)

if new_content != content:
    file_path.write_text(new_content, encoding="utf-8")
    print("Removed auto-submit block")
else:
    print("Pattern not found, trying alternative...")
    
    # Alternative: direct string replacement
    lines = content.split('\n')
    new_lines = []
    skip_until_close = False
    for i, line in enumerate(lines):
        if 'Auto-submit account creation' in line:
            skip_until_close = True
            continue
        if skip_until_close:
            if line.strip() == '}, 500)' and i > 0:
                skip_until_close = False
                continue
            continue
        new_lines.append(line)
    
    new_content = '\n'.join(new_lines)
    if new_content != content:
        file_path.write_text(new_content, encoding="utf-8")
        print("Removed auto-submit block (alternative method)")
    else:
        print("Could not find auto-submit block")
