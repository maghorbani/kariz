# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do NOT** open a public GitHub issue
2. Email: security@kariz.dev (or use GitHub's private vulnerability reporting)
3. Include steps to reproduce and potential impact

We'll acknowledge within 48 hours and aim to release a fix within 7 days for critical issues.

## Security Considerations

KARIZ mounts the Docker socket, which grants significant access to the host. Please review:

- **Docker socket access**: KARIZ never mounts the Docker socket into sibling containers
- **Role-based access**: Only admins can register commands; users can only execute commands their roles allow
- **Session security**: httpOnly, SameSite=Strict cookies; server-side sessions in PostgreSQL
- **Parameter injection**: Command parameters are passed as isolated array elements, never through a shell
- **Secrets**: Environment variable mappings read from containers at runtime — secrets are never stored in KARIZ
