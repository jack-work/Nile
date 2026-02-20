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
          vendorHash = null; # TODO: set after running `go mod vendor`
          subPackages = [ "cmd/nile" ];

          meta = {
            description = "Nile runtime for durable message-driven services";
            mainProgram = "nile";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
          ];
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
