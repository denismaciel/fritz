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

  CGO_ENABLED = "1";
  CGO_CFLAGS = "-I${sqlite.dev}/include";
  CGO_LDFLAGS = "-L${sqlite.out}/lib";

  tags = [ "sqlite_fts5" ];

  vendorHash = "sha256-hSA5DOn/L99ZiuCpadQABp0bLs/+ktLJSeLWGP3T8CA=";

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
