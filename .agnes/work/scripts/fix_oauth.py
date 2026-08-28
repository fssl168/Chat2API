#!/usr/bin/env python3
"""Fix OAuth auto-save in AddAccountDialog.tsx"""

import sys
from pathlib import Path

file_path = Path(r"src/renderer/src/components/providers/AddAccountDialog.tsx")
content = file_path.read_text(encoding="utf-8")

# Find the target section
target = """        setValidationResult({
          valid: true,
          userInfo: result.accountInfo
        })
      } else {"""

replacement = """        setValidationResult({
          valid: true,
          userInfo: result.accountInfo
        })
        
        // Auto-submit account creation after successful OAuth login
        setTimeout(() => {
          handleSubmit()
            .then(() => {
              console.log('[AddAccountDialog] OAuth account created successfully')
            })
            .catch((error) => {
              console.error('[AddAccountDialog] Auto-submit failed:', error)
              setOAuthStatus(error instanceof Error ? error.message : t('providers.saveFailed'))
            })
        }, 500)
      } else {"""

if target in content:
    new_content = content.replace(target, replacement)
    file_path.write_text(new_content, encoding="utf-8")
    print("✅ Successfully added auto-submit after OAuth login")
else:
    print("❌ Target pattern not found")
    sys.exit(1)
