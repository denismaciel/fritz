{
  buildGoModule,
  lib,
  makeWrapper,
  pkg-config,
  ripgrep,
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

  nativeBuildInputs = [
    makeWrapper
    pkg-config
  ];
  buildInputs = [ sqlite ];

  CGO_CFLAGS = "-I${sqlite.dev}/include";
  CGO_LDFLAGS = "-L${sqlite.out}/lib";

  tags = [ "sqlite_fts5" ];

  vendorHash = "sha256-hSA5DOn/L99ZiuCpadQABp0bLs/+ktLJSeLWGP3T8CA=";

  ldflags = [
    "-s"
    "-w"
  ];

  postInstall = ''
    wrapProgram "$out/bin/${mainProgram}" \
      --prefix PATH : ${lib.makeBinPath [ ripgrep ]}
  '';

  meta = with lib; {
    description = "Fritz CLI coding agent";
    inherit mainProgram;
    platforms = platforms.unix;
  };
}
