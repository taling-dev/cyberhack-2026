/// <reference path="./.sst/platform/config.d.ts" />

export default $config({
  app(input) {
    return {
      name: "simaops-ai",
      removal: input?.stage === "production" ? "retain" : "remove",
      protect: input?.stage === "production",
      home: "local",
      providers: {
        oci: {
          region: process.env.OCI_REGION ?? "ap-singapore-1",
          tenancyOcid: process.env.OCI_TENANCY_OCID,
          userOcid: process.env.OCI_USER_OCID,
          fingerprint: process.env.OCI_FINGERPRINT,
          privateKey: process.env.OCI_PRIVATE_KEY,
          version: "3.13.0",
        },
      },
    };
  },

  async run() {
    const stage = $app.stage;
    const compartmentId = process.env.OCI_COMPARTMENT_OCID!;
    if (!compartmentId) {
      throw new Error("OCI_COMPARTMENT_OCID env var is required");
    }

    // ─── Networking ──────────────────────────────────────────────
    const vcn = new oci.core.Vcn("simaops-vcn", {
      compartmentId,
      cidrBlocks: ["10.0.0.0/16"],
      displayName: `simaops-${stage}-vcn`,
      dnsLabel: "simaops",
    });

    const igw = new oci.core.InternetGateway("simaops-igw", {
      compartmentId,
      vcnId: vcn.id,
      enabled: true,
      displayName: `simaops-${stage}-igw`,
    });

    const routeTable = new oci.core.RouteTable("simaops-rt", {
      compartmentId,
      vcnId: vcn.id,
      displayName: `simaops-${stage}-rt`,
      routeRules: [
        {
          destination: "0.0.0.0/0",
          destinationType: "CIDR_BLOCK",
          networkEntityId: igw.id,
        },
      ],
    });

    const securityList = new oci.core.SecurityList("simaops-sl", {
      compartmentId,
      vcnId: vcn.id,
      displayName: `simaops-${stage}-sl`,
      egressSecurityRules: [
        { protocol: "all", destination: "0.0.0.0/0" },
      ],
      ingressSecurityRules: [
        // SSH for debugging (restrict to your IP in production)
        { protocol: "6", source: "0.0.0.0/0", tcpOptions: { min: 22, max: 22 } },
        // HTTP/HTTPS for ingress LB
        { protocol: "6", source: "0.0.0.0/0", tcpOptions: { min: 80, max: 80 } },
        { protocol: "6", source: "0.0.0.0/0", tcpOptions: { min: 443, max: 443 } },
        // Kubernetes API (OKE public endpoint)
        { protocol: "6", source: "0.0.0.0/0", tcpOptions: { min: 6443, max: 6443 } },
        // NodePort range
        { protocol: "6", source: "10.0.0.0/16", tcpOptions: { min: 30000, max: 32767 } },
        // ICMP
        { protocol: "1", source: "0.0.0.0/0" },
        // Internal traffic
        { protocol: "all", source: "10.0.0.0/16" },
      ],
    });

    const subnet = new oci.core.Subnet("simaops-subnet", {
      compartmentId,
      vcnId: vcn.id,
      cidrBlock: "10.0.16.0/24",
      displayName: `simaops-${stage}-lb-subnet`,
      dnsLabel: "lb",
      routeTableId: routeTable.id,
      securityListIds: [securityList.id],
      prohibitPublicIpOnVnic: false,
    });

    const workerSubnet = new oci.core.Subnet("simaops-worker-subnet", {
      compartmentId,
      vcnId: vcn.id,
      cidrBlock: "10.0.17.0/24",
      displayName: `simaops-${stage}-worker-subnet`,
      dnsLabel: "worker",
      routeTableId: routeTable.id,
      securityListIds: [securityList.id],
      prohibitPublicIpOnVnic: false,
    });

    // ─── OKE Cluster ─────────────────────────────────────────────
    const cluster = new oci.containerengine.Cluster("simaops-oke", {
      compartmentId,
      vcnId: vcn.id,
      kubernetesVersion: "v1.36.0",
      name: `simaops-${stage}-oke`,
      type: "BASIC_CLUSTER",
      endpointConfig: {
        subnetId: subnet.id,
        isPublicIpEnabled: true,
      },
      options: {
        serviceLbSubnetIds: [subnet.id],
        kubernetesNetworkConfig: {
          podsCidr: "10.244.0.0/16",
          servicesCidr: "10.96.0.0/16",
        },
      },
    });

    // Get availability domain (use the first one in the region)
    const ads = oci.identity.getAvailabilityDomains({ compartmentId });
    const adName = ads.then((d) => d.availabilityDomains[0].name);

    // Get latest OKE-compatible Oracle Linux 8 image for E4.Flex (matching cluster version)
    const nodePoolOption = oci.containerengine.getNodePoolOption({
      nodePoolOptionId: "all",
      compartmentId,
    });
    const imageId = nodePoolOption.then((o) => {
      const sources = (o.sources ?? []).filter(
        (s) =>
          s.sourceName.includes("Oracle-Linux-8") &&
          s.sourceName.includes("OKE-1.36") &&
          !s.sourceName.includes("aarch64") &&
          !s.sourceName.includes("GPU"),
      );
      return sources[sources.length - 1]?.imageId ?? sources[0]?.imageId ?? "";
    });

    // Cloud-init userdata for worker nodes — enforces NTP sync before the
    // kubelet starts so a node with a misconfigured clock can't join the
    // cluster and serve traffic with broken JWT validation. Oracle Linux 8/9
    // ships chrony by default; this script ensures chronyd is enabled and
    // synced within 30s.
    //
    // Note: existing OKE worker images include `oke-bootstrap.sh`. We use
    // cloud-init's `runcmd` rather than overriding the bootstrap so OCI's
    // managed onboarding still runs.
    const workerUserData = Buffer.from(
      [
        "#cloud-config",
        "runcmd:",
        "  - systemctl enable --now chronyd",
        "  - chronyc waitsync 30 0.1 0 1 || (echo \"NTP failed to sync within 30s\" >&2; exit 1)",
        "  - chronyc tracking",
        "",
      ].join("\n"),
      "utf-8",
    ).toString("base64");

    // ─── Node Pool (VM.Standard.E4.Flex, burstable, AMD) ─────────
    const nodePool = new oci.containerengine.NodePool("simaops-pool", {
      compartmentId,
      clusterId: cluster.id,
      name: `simaops-${stage}-pool`,
      kubernetesVersion: "v1.36.0",
      nodeShape: "VM.Standard.E4.Flex",
      nodeShapeConfig: {
        ocpus: 2,
        memoryInGbs: 16,
      },
      nodeConfigDetails: {
        size: 2,
        placementConfigs: [
          {
            availabilityDomain: adName,
            subnetId: workerSubnet.id,
          },
        ],
      },
      nodeSourceDetails: {
        sourceType: "IMAGE",
        imageId,
        bootVolumeSizeInGbs: "50",
      },
      // Inject cloud-init via nodeMetadata.user_data (base64-encoded).
      nodeMetadata: {
        user_data: workerUserData,
      },
    });

    // ─── Outputs ─────────────────────────────────────────────────
    return {
      stage,
      region: process.env.OCI_REGION ?? "ap-singapore-1",
      clusterOcid: cluster.id,
      vcnId: vcn.id,
      subnetId: subnet.id,
      compartmentId,
      note:
        "After deploy: run `oci ce cluster create-kubeconfig --cluster-id <clusterOcid> --file ~/.kube/config --region ap-singapore-1`, then deploy Helm releases via Task 23.",
    };
  },
});
