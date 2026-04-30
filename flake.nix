{
  description = "fritz coding agent harness";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";

  outputs =
    { self, nixpkgs }:
    let
      lib = nixpkgs.lib;
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          fritz = pkgs.callPackage ./package.nix { };
          fritz-telegram = pkgs.callPackage ./package.nix {
            pname = "fritz-telegram";
            subPackage = "cmd/fritz-telegram";
            mainProgram = "fritz-telegram";
          };
          fritz-slack = pkgs.callPackage ./package.nix {
            pname = "fritz-slack";
            subPackage = "cmd/fritz-slack";
            mainProgram = "fritz-slack";
          };
        in
        {
          inherit fritz fritz-telegram fritz-slack;
          default = fritz;
        }
      );

      apps = forAllSystems (system: {
        fritz = {
          type = "app";
          program = "${self.packages.${system}.fritz}/bin/fritz";
        };
        fritz-telegram = {
          type = "app";
          program = "${self.packages.${system}.fritz-telegram}/bin/fritz-telegram";
        };
        fritz-slack = {
          type = "app";
          program = "${self.packages.${system}.fritz-slack}/bin/fritz-slack";
        };
        default = self.apps.${system}.fritz;
      });

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              gcc
              go
              gopls
              gotools
              pkg-config
              sqlite
            ];
          };
        }
      );
    };
}
