"""Run the Monet Code CLI with ``python -m monet_cli``.

This is also how the self-contained release bundle starts the CLI, so it must
not depend on the package being installed by pip.
"""

from .cli import entrypoint

if __name__ == "__main__":
    entrypoint()
