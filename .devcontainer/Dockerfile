FROM debian:bookworm-slim

ARG USERNAME=developer
ARG USER_UID=1000
ARG USER_GID=${USER_UID}

RUN groupadd --gid "${USER_GID}" "${USERNAME}" \
    && useradd --uid "${USER_UID}" --gid "${USER_GID}" -m "${USERNAME}" \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        bubblewrap \
        ca-certificates \
        curl \
        gcc \
        g++ \
        git \
        libc6-dev \
        libstdc++-12-dev \
        make \
        unzip \
    && rm -rf /var/lib/apt/lists/*

USER ${USERNAME}

ENV PATH="/home/${USERNAME}/.local/bin:/home/${USERNAME}/.local/share/mise/shims:${PATH}"

RUN curl -fsSL https://mise.run | sh \
    && echo 'eval "$(mise activate bash)"' >> "/home/${USERNAME}/.bashrc"

COPY --chown=${USERNAME}:${USERNAME} mise.toml /home/${USERNAME}/.config/mise/config.toml

RUN mise trust /home/${USERNAME}/.config/mise/config.toml \
    && mise install \
    && mise reshim

RUN mkdir -p \
    /home/${USERNAME}/.kiro \
    /home/${USERNAME}/.local/share/kiro-cli \
    /home/${USERNAME}/.codex \
    /home/${USERNAME}/.claude

# Kiro CLI
RUN curl -fsSL https://cli.kiro.dev/install | bash

# OpenAI Codex CLI
RUN curl -fsSL https://chatgpt.com/codex/install.sh | sh

# Claude Code CLI (native installer, no global npm install required)
RUN curl -fsSL https://claude.ai/install.sh | bash

WORKDIR /workspace
