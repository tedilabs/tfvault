package cli

import (
	"fmt"
	"io"
)

// Completion scripts are static: verbs, flags and group subcommands are
// embedded, and profile names are completed dynamically by invoking
// "tfvault profiles" at completion time (its output is stable and plain
// when piped). Protocol verbs are included since operators do call them
// by hand when debugging.

const bashCompletion = `# bash completion for tfvault
# Install: eval "$(tfvault completion bash)"
_tfvault() {
    local cur prev
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    case "$prev" in
        config)
            COMPREPLY=($(compgen -W "show edit" -- "$cur")); return ;;
        help)
            COMPREPLY=($(compgen -W "config" -- "$cur")); return ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur")); return ;;
        --profile)
            COMPREPLY=($(compgen -W "$(tfvault profiles 2>/dev/null | awk '/^\* /{print $2;next} /^  /{print $1}')" -- "$cur")); return ;;
        --config)
            COMPREPLY=($(compgen -f -- "$cur")); return ;;
    esac

    case "$cur" in
        -*)
            COMPREPLY=($(compgen -W "--profile --config --no-color --version --help" -- "$cur")) ;;
        *)
            COMPREPLY=($(compgen -W "get store forget install status config profiles list version help completion" -- "$cur")) ;;
    esac
}
complete -F _tfvault tfvault
`

const zshCompletion = `#compdef tfvault
# zsh completion for tfvault
# Install: tfvault completion zsh > "${fpath[1]}/_tfvault"
#      or: eval "$(tfvault completion zsh)"

_tfvault_profiles() {
    local -a profiles
    profiles=(${(f)"$(tfvault profiles 2>/dev/null | awk '/^\* /{print $2;next} /^  /{print $1}')"})
    _describe 'profile' profiles
}

_tfvault() {
    local -a commands
    commands=(
        'get:print stored credentials for hostname as JSON'
        'store:store credentials read from stdin as JSON'
        'forget:remove stored credentials for hostname'
        'install:symlink the helper into ~/.terraform.d/plugins'
        'status:show plugin link, terraformrc and profile resolution'
        'config:inspect and edit the tfvault configuration'
        'profiles:list configured profiles'
        'list:list hostnames with stored credentials'
        'version:print version information'
        'help:print usage'
        'completion:print a shell completion script'
    )

    _arguments -C \
        '--profile[profile to use]:profile:_tfvault_profiles' \
        '--config[config file path]:file:_files' \
        '--no-color[disable colored output]' \
        '--version[print version information]' \
        '1:command:->cmds' \
        '*::arg:->args'

    case "$state" in
        cmds)
            _describe 'command' commands ;;
        args)
            case "$words[1]" in
                config) _values 'subcommand' 'show[print the effective configuration]' 'edit[open the config file in the configured editor]' ;;
                help) _values 'topic' 'config' ;;
                completion) _values 'shell' 'bash' 'zsh' 'fish' ;;
                install) _values 'flag' '-f[replace an existing non-symlink file]' '--force[replace an existing non-symlink file]' ;;
            esac ;;
    esac
}

if [ "$funcstack[1]" = "_tfvault" ]; then
    _tfvault "$@"
else
    compdef _tfvault tfvault
fi
`

const fishCompletion = `# fish completion for tfvault
# Install: tfvault completion fish > ~/.config/fish/completions/tfvault.fish

function __tfvault_profiles
    tfvault profiles 2>/dev/null | string match -e -r '^\* |^  ' | string replace -r '^\* ' '' | string trim -l | string split -f1 ' '
end

complete -c tfvault -f

complete -c tfvault -n __fish_use_subcommand -a get -d 'print stored credentials for hostname as JSON'
complete -c tfvault -n __fish_use_subcommand -a store -d 'store credentials read from stdin as JSON'
complete -c tfvault -n __fish_use_subcommand -a forget -d 'remove stored credentials for hostname'
complete -c tfvault -n __fish_use_subcommand -a install -d 'symlink the helper into ~/.terraform.d/plugins'
complete -c tfvault -n __fish_use_subcommand -a status -d 'show plugin link, terraformrc and profile resolution'
complete -c tfvault -n __fish_use_subcommand -a config -d 'inspect and edit the tfvault configuration'
complete -c tfvault -n __fish_use_subcommand -a profiles -d 'list configured profiles'
complete -c tfvault -n __fish_use_subcommand -a list -d 'list hostnames with stored credentials'
complete -c tfvault -n __fish_use_subcommand -a version -d 'print version information'
complete -c tfvault -n __fish_use_subcommand -a help -d 'print usage'
complete -c tfvault -n __fish_use_subcommand -a completion -d 'print a shell completion script'

complete -c tfvault -n '__fish_seen_subcommand_from config' -a 'show edit'
complete -c tfvault -n '__fish_seen_subcommand_from help' -a config
complete -c tfvault -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
complete -c tfvault -n '__fish_seen_subcommand_from install' -s f -l force -d 'replace an existing non-symlink file'

complete -c tfvault -l profile -x -a '(__tfvault_profiles)' -d 'profile to use'
complete -c tfvault -l config -r -d 'config file path'
complete -c tfvault -l no-color -d 'disable colored output'
complete -c tfvault -l version -d 'print version information'
`

// runCompletion prints the completion script for the requested shell.
func runCompletion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "tfvault: completion: expected exactly one shell: bash, zsh, fish")
		return 1
	}
	switch args[0] {
	case "bash":
		fmt.Fprint(stdout, bashCompletion)
	case "zsh":
		fmt.Fprint(stdout, zshCompletion)
	case "fish":
		fmt.Fprint(stdout, fishCompletion)
	default:
		fmt.Fprintf(stderr, "tfvault: completion: unsupported shell %q (expected bash, zsh, fish)\n", args[0])
		return 1
	}
	return 0
}
