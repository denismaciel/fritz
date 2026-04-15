{
  buildGoModule,
  lib,
  pkg-config,
  sqlite,
  pname ? "fritz",
  subPackage ? "cmd/fritz",
  mainProgram ? pname,
}:
buildGoModule {
  inherit pname;
  version = "0.1.0";

  src = ./.;

  subPackages = [ subPackage ];

  nativeBuildInputs = [ pkg-config ];
  buildInputs = [ sqlite ];

  tags = [ "sqlite_fts5" ];

  vendorHash = "sha256-J5ra0jhG2uBL0B+IaPggHR9FaHRFLsuk6GJrsslKsx0=";

  ldflags = [
    "-s"
    "-w"
  ];

  meta = with lib; {
    description = "Fritz CLI coding agent";
    inherit mainProgram;
    platforms = platforms.unix;
  };
}
