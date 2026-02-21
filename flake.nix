{
  description = "Nile: durable, sandboxed, message-driven services";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils, ... }:
    let
      # NixOS module (system-independent)
      nixosModule = import ./modules/nile.nix;
    in
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "nile";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-AYissG+4cwerkFqpCY+efDrusoDXYE8RCnKTvxihUUc=";
          subPackages = [ "cmd/nile" ];

          meta = {
            description = "Nile runtime for durable message-driven services";
            mainProgram = "nile";
          };
        };

        packages.counter-service = pkgs.buildGoModule {
          pname = "counter-service";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-AYissG+4cwerkFqpCY+efDrusoDXYE8RCnKTvxihUUc=";
          subPackages = [ "examples/counter-service" ];
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
          ];

          packages = [
            self.packages.${system}.default
            self.packages.${system}.counter-service
          ];

          NILE_DEMO_DIR = "/tmp/nile-demo";

          shellHook = ''
            mkdir -p $NILE_DEMO_DIR/{state,retain,dead,run,stream}
            echo "Nile dev shell"
            echo ""
            echo "  T1: nile run demo --binary counter-service --data-dir $NILE_DEMO_DIR"
            echo "  T2: nile watch --data-dir $NILE_DEMO_DIR demo"
            echo "  T3: tail -f $NILE_DEMO_DIR/state/activity.log"
            echo ""
            echo "  Send: nile send --data-dir $NILE_DEMO_DIR demo \"hello world\""
          '';
        };
      }
    ) // {
      nixosModules.default = nixosModule;

      templates.service = {
        path = ./templates/service;
        description = "A new Nile service (copt)";
      };
    };
}
