{ config, lib, pkgs, ... }:

let
  cfg = config.services.nile;
in
{
  options.services.nile = {
    package = lib.mkOption {
      type = lib.types.package;
      description = "The Nile runtime package.";
    };

    copts = lib.mkOption {
      type = lib.types.attrsOf (lib.types.submodule ({ name, ... }: {
        options = {
          package = lib.mkOption {
            type = lib.types.package;
            description = "The neb (service) package for this copt.";
          };

          retention = {
            maxMessages = lib.mkOption {
              type = lib.types.int;
              default = 10000;
              description = "Maximum consumed messages before triggering retention.";
            };

            maxBytes = lib.mkOption {
              type = lib.types.int;
              default = 10485760;
              description = "Maximum total log bytes before triggering retention.";
            };
          };

          segmentSize = lib.mkOption {
            type = lib.types.int;
            default = 1048576;
            description = "Bytes per WAL segment before rolling to a new one.";
          };

          messageTimeout = lib.mkOption {
            type = lib.types.int;
            default = 60;
            description = "Neb response timeout in seconds.";
          };

          maxRetries = lib.mkOption {
            type = lib.types.int;
            default = 3;
            description = "Retries before dead-lettering a message.";
          };

          maxDepth = lib.mkOption {
            type = lib.types.int;
            default = 0;
            description = "Max unprocessed messages (0 = unlimited). Phase 2: HTTP 429 when exceeded.";
          };

          sandbox = {
            extraReadPaths = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = [ ];
              description = "Additional read-only paths for the neb sandbox.";
            };

            extraWritePaths = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = [ ];
              description = "Additional read-write paths for the neb sandbox.";
            };

            network = lib.mkOption {
              type = lib.types.bool;
              default = false;
              description = "Whether the neb is allowed network access.";
            };
          };
        };
      }));
      default = { };
      description = "Attribute set of copts to run.";
    };
  };

  config = lib.mkIf (cfg.copts != { }) {
    systemd.services = lib.mapAttrs' (name: copt:
      lib.nameValuePair "nile-${name}" {
        description = "Nile copt: ${name}";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];

        serviceConfig = {
          Type = "simple";
          ExecStart = lib.concatStringsSep " " [
            "${cfg.package}/bin/nile"
            "run"
            name
            "--binary"
            "${copt.package}/bin/${name}"
            "--data-dir"
            "/var/lib/nile/${name}"
            "--max-messages"
            (toString copt.retention.maxMessages)
            "--max-bytes"
            (toString copt.retention.maxBytes)
            "--segment-size"
            (toString copt.segmentSize)
            "--message-timeout"
            (toString copt.messageTimeout)
            "--max-retries"
            (toString copt.maxRetries)
          ] ++ lib.optionals (copt.maxDepth > 0) [
            "--max-depth"
            (toString copt.maxDepth)
          ];
          Restart = "on-failure";
          RestartSec = 5;

          # Systemd sandboxing (defense in depth alongside Landlock)
          StateDirectory = "nile/${name}";
          ProtectSystem = "strict";
          ProtectHome = true;
          PrivateTmp = true;
          NoNewPrivileges = true;
          ReadWritePaths = [
            "/var/lib/nile/${name}"
          ] ++ copt.sandbox.extraWritePaths;
          ReadOnlyPaths = [
            "/nix/store"
          ] ++ copt.sandbox.extraReadPaths;
        };
      }
    ) cfg.copts;
  };
}
