/// <reference path="./.sst/platform/config.d.ts" />

export default $config({
  app(input) {
    return {
      name: "simaops-ai",
      removal: input?.stage === "production" ? "retain" : "remove",
      protect: input?.stage === "production",
      providers: {
        gcp: {
          project: "taling-cyberhack",
          region: "asia-southeast2",
          zone: "asia-southeast2-a",
        },
        kubernetes: "4.18.3",
      },
    };
  },

  async run() {
    const stage = $app.stage;
    const project = "taling-cyberhack";
    const region = "asia-southeast2";
    const zone = "asia-southeast2-a";

    // ─── GKE Cluster ─────────────────────────────────────────────
    const network = new gcp.compute.Network("simaops-vpc", {
      autoCreateSubnetworks: false,
    });

    const subnet = new gcp.compute.Subnetwork("simaops-subnet", {
      network: network.id,
      ipCidrRange: "10.0.0.0/20",
      region,
    });

    const cluster = new gcp.container.Cluster("simaops-gke", {
      location: region,
      network: network.id,
      subnetwork: subnet.id,
      initialNodeCount: 1,
      removeDefaultNodePool: true,
      deletionProtection: stage === "production",
    });

    // System node pool (platform services)
    const systemPool = new gcp.container.NodePool("system-pool", {
      cluster: cluster.name,
      location: region,
      nodeCount: 3,
      nodeConfig: {
        machineType: "e2-standard-4",
        oauthScopes: ["https://www.googleapis.com/auth/cloud-platform"],
        labels: { pool: "system" },
      },
    });

    // App node pool (application workloads, autoscaling)
    const appPool = new gcp.container.NodePool("app-pool", {
      cluster: cluster.name,
      location: region,
      autoscaling: {
        minNodeCount: 3,
        maxNodeCount: 6,
      },
      nodeConfig: {
        machineType: "e2-standard-8",
        oauthScopes: ["https://www.googleapis.com/auth/cloud-platform"],
        labels: { pool: "app" },
      },
    });

    // Static IP for ingress
    const staticIp = new gcp.compute.GlobalAddress("simaops-ingress-ip", {
      addressType: "EXTERNAL",
    });

    // ─── Outputs ─────────────────────────────────────────────────
    return {
      stage,
      project,
      region,
      clusterName: cluster.name,
      staticIp: staticIp.address,
      // DNS: use <ip>.sslip.io until a real domain is configured
      frontendUrl: $interpolate`https://app.${staticIp.address}.sslip.io`,
      apiUrl: $interpolate`https://api.${staticIp.address}.sslip.io`,
      keycloakUrl: $interpolate`https://auth.${staticIp.address}.sslip.io`,
      grafanaUrl: $interpolate`https://grafana.${staticIp.address}.sslip.io`,
      note: "Helm releases deployed separately via deploy-staging workflow after cluster is ready",
    };
  },
});
