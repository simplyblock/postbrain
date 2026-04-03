Name:           postbrain-client
Version:        0.0.0
Release:        1%{?dist}
Summary:        Postbrain CLI client

License:        Apache-2.0
URL:            https://github.com/simplyblock/postbrain
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang

%description
Postbrain client provides command-line utilities for interacting with a Postbrain server.

%prep
%autosetup

%build
make build

%install
install -D -m 0755 postbrain-cli %{buildroot}%{_bindir}/postbrain-cli

%files
%license LICENSE
%doc README.md
%{_bindir}/postbrain-cli

%changelog
* Fri Apr 03 2026 Postbrain Team <opensource@simplyblock.io> - 0.0.0-1
- Initial client package scaffold
