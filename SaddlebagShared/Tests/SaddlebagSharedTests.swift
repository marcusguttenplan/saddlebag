import Testing
@testable import SaddlebagShared

@Test func testSaddlebagSharedImport() async throws {
    // Verify the shared package types are accessible
    let profile = AWSProfile(
        name: "test",
        ssoSessionName: "test-session",
        accountId: "123456789012",
        roleName: "AdministratorAccess"
    )
    #expect(profile.isAdmin == true)
    #expect(profile.roleLabel == "Admin")
}
