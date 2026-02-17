# Vault server configuration for local testing.
# In dev mode Vault uses in-memory storage and its own listener, so we only
# provide supplementary settings here. Do NOT define a "listener" or "storage"
# block – dev mode already binds to VAULT_DEV_LISTEN_ADDRESS and uses
# in-memory storage. Duplicating the listener causes "address already in use".

# Disable mlock for containers (avoids IPC_LOCK requirement in some runtimes).
disable_mlock = true

# UI enabled for debugging.
ui = true
