//! Starts `buti lsp`, which shows buti's review comments as diagnostics.

use zed_extension_api::{self as zed, LanguageServerId, Result};

struct Buti;

impl zed::Extension for Buti {
    fn new() -> Self {
        Buti
    }

    fn language_server_command(
        &mut self,
        _language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let command = worktree
            .which("buti")
            .ok_or_else(|| "buti not found on PATH".to_string())?;
        Ok(zed::Command {
            command,
            args: vec!["lsp".to_string()],
            // The shell's environment, so buti finds `but` and `git` where the user's PATH has them.
            env: worktree.shell_env(),
        })
    }
}

zed::register_extension!(Buti);
